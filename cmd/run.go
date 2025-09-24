package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spigell/hh-responder/internal/headhunter"
	"github.com/spigell/hh-responder/internal/logger"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	forceFlagSetMsg           = "force flag is set"
	PromptYes                 = "Yes"
	PromptNo                  = "No"
	PromptBack                = "back"
	PromptReportByEmployers   = "Report by employers"
	PromptManualApply         = "Apply vacancies in manual mode"
	PromptAppendToExcludeFile = "Append all vacancies to exclude file"
	PromptVacanciesToFile     = "Dump vacancies to file"
)

var errExit = errors.New("exit requested")

var prompt = promptui.Select{
	Label: "Procced?",
	Items: []string{PromptYes, PromptNo, PromptReportByEmployers, PromptManualApply, PromptVacanciesToFile},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the hh-responder main command",
	Run: func(cmd *cobra.Command, _ []string) {
		run(cmd)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().BoolP("do-not-exclude-applied", "f", false, "do not exclude vacancies if already applied")
	runCmd.Flags().BoolP("auto-aprove", "y", false, "do not ask for confirmation if found suitable vacancies")
	runCmd.Flags().StringP("exclude-file", "e", "", "special file with vacancies to exclude. Default is unset.")

	viper.BindPFlag("exclude-file", runCmd.Flags().Lookup("exclude-file"))
}

// run is the main command for the cli.
func run(cmd *cobra.Command) {
	ctx := context.Background()

	logger, err := logger.New(viper.GetBool("json"), viper.GetBool("debug"))
	if err != nil {
		log.Fatalf("creating a logger: %s", err)
	}

	config, err := getConfig()
	if err != nil {
		logger.Fatal("getting a config", zap.Error(err))
	}

	logger.Info("starting the hh-responder", zap.String("version", version))

	// do not bother error since there is a valid parseable config
	pretty, _ := json.MarshalIndent(config, "", "  ")
	logger.Debug(fmt.Sprintf("starting with config: \n %s", pretty))

	hh := headhunter.New(ctx, logger, os.Getenv(hhTokenEnvVar))

	if config != nil && config.UserAgent != "" {
		hh.UserAgent = config.UserAgent
	}

	logger.Info("starting the search", zap.String("search", config.Search.Text))

	vacancies, err := getVacancies(hh, config, cmd, logger)
	if err != nil {
		logger.Fatal("getting available vacancies", zap.Error(err))
	}

	if vacancies.Len() == 0 {
		logger.Info("exiting", zap.String("reason", "no vacancies left"))
		return
	}

	action := PromptYes
	for {
		var err error
		if cmd.Flag("auto-aprove").Value.String() == "false" {
			_, action, err = prompt.Run()
			if err != nil {
				logger.Fatal("exiting", zap.Error(err))
			}
		}

		logger.Info("current list of vacancies", zap.Int("count", vacancies.Len()))

		if err := handleAction(action, hh, logger, config, vacancies); err != nil {
			if errors.Is(err, errExit) {
				return
			}
			logger.Fatal("exiting", zap.Error(err))
		}
	}
}

func handleAction(action string, hh *headhunter.Client, logger *zap.Logger, config *Config, vacancies *headhunter.Vacancies) error {
	switch action {
	case PromptYes:
		return apply(hh, *logger, config.Apply.Resume, vacancies, config.Apply.Message)
	case PromptNo:
		logger.Info("exiting", zap.String("reason", "got no from prompt"))
		return errExit
	case PromptManualApply:
		return manualApply(hh, logger, config, vacancies)
	case PromptReportByEmployers:
		pretty, _ := json.MarshalIndent(vacancies.ReportByEmployer(), "", "  ")
		logger.Info(string(pretty), zap.Int("vacancies count", vacancies.Len()))
		return nil
	case PromptVacanciesToFile:
		filename, err := vacancies.DumpToTmpFile()
		if err != nil {
			return fmt.Errorf("dump results to file: %w", err)
		}
		logger.Info("dumping result to file", zap.String("filename", filename))
		return nil
	default:
		return fmt.Errorf("invalid action: %s", action)
	}
}

func manualApply(hh *headhunter.Client, logger *zap.Logger, config *Config, vacancies *headhunter.Vacancies) error {
	for {
		items := make([]string, 0)
		v := make([]*headhunter.Vacancy, 0)

		for _, vc := range vacancies.Items {
			items = append(items, fmt.Sprintf("%s %s / %s / %s",
				vc.ID, vc.Name, vc.Employer.Name, vc.AlternateURL),
			)
		}

		excludeFile := viper.GetString("exclude-file")
		if excludeFile != "" && vacancies.Len() != 0 {
			items = append(items, PromptAppendToExcludeFile)
		}

		vacancyPrompt := promptui.Select{
			Label: "Choose a vacancy and press ENTER",
			Items: append(items, PromptBack),
		}

		_, vacancySelected, err := vacancyPrompt.Run()
		if err != nil {
			return err
		}

		switch vacancySelected {
		case PromptBack:
			return nil
		case PromptAppendToExcludeFile:
			excluded, err := headhunter.GetExludedVacanciesFromFile(excludeFile)
			if err != nil {
				return err
			}

			excluded.Append(vacancies.ToExcluded())

			if err = excluded.ToFile(excludeFile); err != nil {
				return err
			}

			logger.Info("appended to exlude file", zap.String("filename", excludeFile))

			vacancies.Exclude(headhunter.VacancyIDField, excluded.VacanciesIDs())
		default:
			vacancyID := strings.Split(vacancySelected, " ")[0]

			v = append(v, vacancies.FindByID(vacancyID))

			if v[0] == nil {
				return fmt.Errorf("there is no such vacancy id %s", vacancyID)
			}

			if err = apply(hh, *logger, config.Apply.Resume, &headhunter.Vacancies{Items: v}, config.Apply.Message); err != nil {
				return err
			}

			vacancies.Exclude(headhunter.VacancyIDField, []string{vacancyID})
		}
	}
}

func apply(hh *headhunter.Client, logger zap.Logger, resumeName string, vacancies *headhunter.Vacancies, message string) error {
	resumes, err := hh.GetMineResumes()
	if err != nil {
		return err
	}

	logger.Info("getting mine resumes", zap.Int("count", resumes.Len()))

	resume := resumes.FindByTitle(resumeName)

	if resume == nil {
		logger.Fatal("resume with given title not found",
			zap.Any("existed resumes titles", resumes.Titles()),
			zap.String("resume title", resumeName),
		)
	}

	err = hh.Apply(resume, vacancies, message)
	if err != nil {
		return err
	}

	logger.Info("successfully applied to vacancies", zap.Int("count", vacancies.Len()))

	return nil
}

// getVacancies returns a list of vacancies that match the config.
// TODO: need refactoring.
func getVacancies(hh *headhunter.Client, config *Config, cmd *cobra.Command, logger *zap.Logger) (*headhunter.Vacancies, error) {
	results, err := hh.Search(config.Search)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	logger.Info("getting vacancies", zap.Int("count", results.Len()))

	negotiations, err := hh.GetNegotiations()
	if err != nil {
		return nil, fmt.Errorf("get my negotiations: %w", err)
	}

	logger.Info("excluding vacancies with test. It is impossible to apply them",
		zap.Any("excluded vacancies", results.ExcludeWithTest()),
		zap.Int("vacancies left", results.Len()),
	)

	if cmd.Flag("do-not-exclude-applied").Value.String() == "true" {
		logger.Info("ignoring already applied vacancies", zap.String("reason", forceFlagSetMsg))
	} else {
		excluded := results.Exclude(headhunter.VacancyIDField, negotiations.VacanciesIDs())
		logger.Info("excluding vacancies based on my negotiations",
			zap.Any("excluded vacancies", excluded),
			zap.Int("vacancies left", results.Len()),
		)
	}

	if config.Apply.Exclude != nil && len(config.Apply.Exclude.Employers) > 0 {
		excluded := results.Exclude(headhunter.VacancyEmployerIDField, config.Apply.Exclude.Employers)
		logger.Info("excluding vacancies by employers",
			zap.Any("excluded employers", config.Apply.Exclude.Employers),
			zap.Any("excluded vacancies", excluded),
			zap.Int("vacancies left", results.Len()),
		)
	}
	excludeFile := viper.GetString("exclude-file")
	if excludeFile != "" {
		excluded, err := headhunter.GetExludedVacanciesFromFile(excludeFile)
		if err != nil {
			return nil, fmt.Errorf("getting exluded vacancies from file: %w", err)
		}

		excludedVacancies := results.Exclude(headhunter.VacancyIDField, excluded.VacanciesIDs())
		logger.Info("excluding vacancies based on exclude file",
			zap.Any("excluded vacancies", excludedVacancies),
			zap.Int("vacancies left", results.Len()),
		)
	}

	return results, nil
}
