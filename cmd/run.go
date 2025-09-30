package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/ai/gemini"
	"github.com/spigell/hh-responder/internal/filtering"
	"github.com/spigell/hh-responder/internal/headhunter"
	"github.com/spigell/hh-responder/internal/logger"
	"github.com/spigell/hh-responder/internal/secrets"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	PromptYes                 = "Yes"
	PromptNo                  = "No"
	PromptBack                = "back"
	PromptReportByEmployers   = "Report by employers"
	PromptManualApply         = "Apply vacancies in manual mode"
	PromptAppendToExcludeFile = "Append all vacancies to exclude file"
	PromptVacanciesToFile     = "Dump vacancies to file"
	defaultFallbackMessage    = "Hello! I would like to apply for this vacancy."
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

	if config == nil {
		logger.Fatal("config is required")
	}

	if config.Apply == nil || config.Apply.Resume == "" {
		logger.Fatal("resume title is required under apply.resume to evaluate and apply to vacancies")
	}

	token, err := resolveToken(config)
	if err != nil {
		logger.Fatal(
			"loading headhunter token",
			zap.Error(err),
			zap.String("hint", "set HH_TOKEN_FILE environment variable or the 'token-file' key in the configuration file"),
		)
	}

	hh := headhunter.New(ctx, token, logger)

	if config.UserAgent != "" {
		hh.UserAgent = config.UserAgent
	}

	resumes, err := hh.GetMineResumes()
	if err != nil {
		logger.Fatal("getting mine resumes", zap.Error(err))
	}

	logger.Info("getting mine resumes", zap.Int("count", resumes.Len()))

	selectedResume := resumes.FindByTitle(config.Apply.Resume)
	if selectedResume == nil {
		logger.Fatal("resume with given title not found",
			zap.Any("existed resumes titles", resumes.Titles()),
			zap.String("resume title", config.Apply.Resume),
		)
	}

	logger.Info("starting the search", zap.String("search", config.Search.Text))

	vacancies, err := getVacancies(hh, config, logger)
	if err != nil {
		logger.Fatal("getting available vacancies", zap.Error(err))
	}

	if vacancies.Len() == 0 {
		logger.Info("exiting", zap.String("reason", "no vacancies found"))
		return
	}

	filters := prepareFilters(ctx, cmd, hh, config, selectedResume, logger)

	filtered, err := filters.RunFilters(ctx, vacancies)
	if err != nil {
		logger.Fatal("filtering failed", zap.Error(err))
	}
	vacancies = filtered

	if vacancies.Len() == 0 {
		logger.Info("exiting", zap.String("reason", "no vacancies left after filters"))
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

		if err := handleAction(action, hh, logger, config, vacancies, selectedResume); err != nil {
			if errors.Is(err, errExit) {
				return
			}
			logger.Fatal("exiting", zap.Error(err))
		}
	}
}

func handleAction(action string, hh *headhunter.Client, logger *zap.Logger, config *Config, vacancies *headhunter.Vacancies, resume *headhunter.Resume) error {
	switch action {
	case PromptYes:
		return apply(hh, *logger, resume, vacancies, config.Apply.Message)
	case PromptNo:
		logger.Info("exiting", zap.String("reason", "got no from prompt"))
		return errExit
	case PromptManualApply:
		return manualApply(hh, logger, config, vacancies, resume)
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

func resolveToken(config *Config) (string, error) {
	if config == nil {
		return "", errors.New("config is required")
	}

	tokenFile := strings.TrimSpace(config.TokenFile)
	if tokenFile == "" {
		tokenFile = strings.TrimSpace(viper.GetString("token-file"))
	}

	if tokenFile == "" {
		return "", errors.New("headhunter token file is not configured")
	}

	return secrets.Load(secrets.Source{
		Name: "headhunter token",
		File: tokenFile,
	})
}

func manualApply(hh *headhunter.Client, logger *zap.Logger, config *Config, vacancies *headhunter.Vacancies, resume *headhunter.Resume) error {
	for {
		items := make([]string, 0)
		v := make([]*headhunter.Vacancy, 0)

		for _, vc := range vacancies.Items {
			label := fmt.Sprintf("%s %s / %s / %s",
				vc.ID, vc.Name, vc.Employer.Name, vc.AlternateURL,
			)

			items = append(items, label)
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

			if err = apply(hh, *logger, resume, &headhunter.Vacancies{Items: v}, config.Apply.Message); err != nil {
				return err
			}

			vacancies.Exclude(headhunter.VacancyIDField, []string{vacancyID})
		}
	}
}

func apply(hh *headhunter.Client, logger zap.Logger, resume *headhunter.Resume, vacancies *headhunter.Vacancies, defaultMessage string) error {
	for _, vacancy := range vacancies.Items {

		message := vacancy.AI.Message
		if message == "" {
			message = defaultMessage
		}

		if message == "" {
			message = defaultFallbackMessage
			logger.Warn("falling back to default built-in message",
				zap.String("vacancy_id", vacancy.ID),
				zap.String("hint", "specify message in apply section"),
			)
		}

		if err := hh.ApplyWithMessage(resume, vacancy, message); err != nil {
			return err
		}

		logger.Info("successfully applied to vacancy",
			zap.String("vacancy_id", vacancy.ID),
			zap.String("vacancy_name", vacancy.Name),
		)
	}

	logger.Info("successfully applied to vacancies", zap.Int("count", vacancies.Len()))
	return nil
}

func newAIMatcher(ctx context.Context, cfg *AIConfig, logger *zap.Logger) (ai.Matcher, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider != "" && provider != "gemini" {
		return nil, fmt.Errorf("unsupported ai provider: %s", cfg.Provider)
	}

	apiKey, err := secrets.Load(secrets.Source{
		Name: "gemini api key",
		File: cfg.Gemini.APIKeyFile,
	})
	if err != nil {
		return nil, fmt.Errorf("%w (set ai.gemini.api-key-file or GEMINI_API_KEY_FILE)", err)
	}

	genLogger := logger.With(
		zap.String("provider", "gemini"),
		zap.String("model", cfg.Gemini.Model),
		zap.Int("ai_retry_attempts", cfg.Gemini.MaxRetries),
	)

	generator, err := gemini.NewGenerator(ctx, apiKey, cfg.Gemini.Model, cfg.Gemini.MaxRetries, genLogger)
	if err != nil {
		return nil, err
	}

	minScore := cfg.MinimumFitScore
	if minScore < 0 {
		minScore = 0
	}

	matcherLogger := logger.With(
		zap.String("provider", "gemini"),
		zap.String("model", cfg.Gemini.Model),
		zap.Float64("minimum_fit_score", minScore),
	)

	matcher := gemini.NewMatcher(generator, minScore, cfg.Gemini.MaxLogLength, matcherLogger)

	return matcher, nil
}

// getVacancies returns a list of vacancies that match the config.
func getVacancies(hh *headhunter.Client, config *Config, logger *zap.Logger) (*headhunter.Vacancies, error) {
	results, err := hh.Search(config.Search)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	logger.Info("getting vacancies", zap.Int("count", results.Len()))
	return results, nil
}

func prepareFilters(ctx context.Context, cmd *cobra.Command, hh *headhunter.Client, config *Config, resume *headhunter.Resume, logger *zap.Logger) *filtering.Filtering {
	aiFilter, err := prepareAIFilter(ctx, hh, config.AI, resume, logger, config.ExcludeFile)
	if err != nil {
		logger.Warn("skipping AI filter", zap.Error(err))
	}

	steps := []filtering.Filter{
		filtering.NewWithTest(),
		prepareAppliedHistoryFilter(cmd, hh, logger),
		filtering.NewExludedEmployers(config.Apply.Exclude.Employers),
		filtering.NewExcludeFile(config.ExcludeFile),
	}

	if !aiFilter.IsEnabled() {
		steps = append(steps, aiFilter)
	}

	return filtering.New(steps, logger)
}

func prepareAppliedHistoryFilter(cmd *cobra.Command, client *headhunter.Client, logger *zap.Logger) filtering.Filter {
	ignore := false
	if cmd != nil {
		flag := cmd.Flag("do-not-exclude-applied")
		if flag != nil && strings.EqualFold(flag.Value.String(), "true") {
			ignore = true
		}
	}

	cfg := &filtering.AppliedHistoryConfig{Ignore: ignore}
	deps := &filtering.AppliedHistoryDeps{
		HH:     client,
		Logger: logger,
	}

	return filtering.NewAppliedHistory(cfg, deps)
}

func prepareAIFilter(ctx context.Context, client *headhunter.Client, config *AIConfig, resume *headhunter.Resume, logger *zap.Logger, excludeFile string) (filtering.Filter, error) {
	if config == nil || !config.Enabled {
		return filtering.NewAIFit(&filtering.AIFitFilterConfig{
			Enabled: false,
		}, nil), nil
	}

	if config.Gemini == nil {
		return nil, fmt.Errorf("gemini configuration is required when ai filter is enabled")
	}

	aiConfig := &filtering.AIFitFilterConfig{
		Enabled:         config.Enabled,
		Provider:        config.Provider,
		MinimumFitScore: config.MinimumFitScore,
		Gemini: &filtering.AIGeminiConfig{
			Model:        config.Gemini.Model,
			MaxRetries:   config.Gemini.MaxRetries,
			MaxLogLength: config.Gemini.MaxLogLength,
		},
	}

	matcher, err := newAIMatcher(ctx, config, logger)
	if err != nil {
		return nil, fmt.Errorf("building ai matcher: %w", err)
	}

	return filtering.NewAIFit(aiConfig, &filtering.AIFitFilterDeps{
		Logger:      logger,
		HH:          client,
		Resume:      resume,
		Matcher:     matcher,
		ExcludeFile: excludeFile,
	}), nil
}
