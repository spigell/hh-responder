package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/spigell/hh-responder/internal/headhunter"
	"github.com/spigell/hh-responder/internal/logger"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)
const (
	forceFlagSetMsg         = "force flag is set"
	PromptYes               = "Yes"
	PromptNo                = "No"
	PromptReportByEmployers = "Report by employers"
	PromptVacanciesToFile   = "Dump vacancies to file"
)

var prompt = promptui.Select{
	Label: "Procced?",
	Items: []string{PromptYes, PromptNo, PromptReportByEmployers, PromptVacanciesToFile},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the hh-responder main command",
	Run: func(cmd *cobra.Command, args []string) {
		run(cmd)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}

// run is the main command for the cli.
func run(cmd *cobra.Command) {
	ctx := context.Background()

	logger, err := logger.New(viper.GetBool("json"), viper.GetBool("debug"))
	if err != nil {
		log.Fatal(fmt.Sprintf("creating a logger: %s", err))
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

	vacancies, err := getVacancies(hh, config, cmd, logger)
	if err != nil {
		logger.Fatal("getting avaliable vacancies", zap.Error(err))
	}

	if vacancies.Len() == 0 {
		logger.Info("exiting", zap.String("reason", "no vacancies left"))
		return
	}

	// main loop
	for {
		_, result, err := prompt.Run()
		if err != nil {
			logger.Fatal("exiting", zap.Error(err))
		}

		switch result {
		case PromptYes:
			resumes, err := hh.GetMineResumes()
			if err != nil {
				logger.Fatal("getting my resumes", zap.Error(err))
			}

			logger.Info("getting mine resumes", zap.Int("count", resumes.Len()))

			resume := resumes.FindByTitle(config.Apply.Resume)

			if resume == nil {
				logger.Fatal("resume with given title not found",
					zap.Any("existed resumes titles", resumes.Titles()),
					zap.String("resume title", config.Apply.Resume),
				)
			}

			err = hh.Apply(resume, vacancies, config.Apply.Message)

			if err != nil {
				logger.Fatal("appling to vacancies", zap.Error(err))
			}

			return

		case PromptNo:
			logger.Info("exiting", zap.String("reason", "got no from prompt"))
			return

		case PromptReportByEmployers:
			pretty, _ := json.MarshalIndent(vacancies.ReportByEmployer(), "", "  ")
			logger.Info(string(pretty), zap.Int("vacancies count", vacancies.Len()))

		case PromptVacanciesToFile:
			filename, err := vacancies.DumpToTmpFile()
			if err != nil {
				logger.Fatal("dump results to file", zap.Error(err))
			}

			logger.Info("dumping result to file", zap.String("filename", filename))

		default:
			logger.Fatal("something wrong happen. exiting")
		}
	}
}

// getVacancies returns a list of vacancies that match the config.
// TODO: need refactoring.
func getVacancies(hh *headhunter.Client, config *Config, cmd *cobra.Command, logger *zap.Logger) (*headhunter.Vacancies, error ){
	results, err := hh.Search(config.Search)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	logger.Info("getting vacancies", zap.Int("count", results.Len()))

	negotiations, err := hh.GetNegotiations()
	if err != nil {
		return nil, fmt.Errorf("get my negotiations: %s", err)
	}

	logger.Info("excluding vacancies with test. It is impossible to apply them",
		zap.Any("excluded vacancies", results.ExcludeWithTest()),
		zap.Int("vacancies left", results.Len()),
	)

	if cmd.Flag("force").Value.String() == "true" {
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

	return results, nil
}
