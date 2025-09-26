package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spigell/hh-responder/internal/ai"
	"github.com/spigell/hh-responder/internal/ai/gemini"
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

	token, err := resolveToken(config)
	if err != nil {
		logger.Fatal(
			"loading headhunter token",
			zap.Error(err),
			zap.String("hint", "set HH_TOKEN_FILE environment variable or the 'token-file' key in the configuration file"),
		)
	}

	hh := headhunter.New(ctx, logger, token)

	if config != nil && config.UserAgent != "" {
		hh.UserAgent = config.UserAgent
	}

	if config.Apply == nil || config.Apply.Resume == "" {
		logger.Fatal("resume title is required under apply.resume to evaluate and apply to vacancies")
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

	vacancies, err := getVacancies(hh, config, cmd, logger)
	if err != nil {
		logger.Fatal("getting available vacancies", zap.Error(err))
	}

	if vacancies.Len() == 0 {
		logger.Info("exiting", zap.String("reason", "no vacancies left"))
		return
	}

	aiAssessments, err := prepareAIEvaluation(ctx, logger, config, hh, selectedResume, vacancies)
	if err != nil {
		logger.Fatal("evaluating vacancies with AI", zap.Error(err))
	}

	if vacancies.Len() == 0 {
		logger.Info("exiting", zap.String("reason", "no vacancies match resume according to AI"))
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

		if err := handleAction(action, hh, logger, config, vacancies, selectedResume, aiAssessments); err != nil {
			if errors.Is(err, errExit) {
				return
			}
			logger.Fatal("exiting", zap.Error(err))
		}
	}
}

func handleAction(action string, hh *headhunter.Client, logger *zap.Logger, config *Config, vacancies *headhunter.Vacancies, resume *headhunter.Resume, assessments map[string]*ai.FitAssessment) error {
	switch action {
	case PromptYes:
		return apply(hh, *logger, resume, vacancies, config.Apply.Message, assessments)
	case PromptNo:
		logger.Info("exiting", zap.String("reason", "got no from prompt"))
		return errExit
	case PromptManualApply:
		return manualApply(hh, logger, config, vacancies, resume, assessments)
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

	tokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("reading token file %q: %w", tokenFile, err)
	}

	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return "", fmt.Errorf("token file %q is empty", tokenFile)
	}

	return token, nil
}

func manualApply(hh *headhunter.Client, logger *zap.Logger, config *Config, vacancies *headhunter.Vacancies, resume *headhunter.Resume, assessments map[string]*ai.FitAssessment) error {
	for {
		items := make([]string, 0)
		v := make([]*headhunter.Vacancy, 0)

		for _, vc := range vacancies.Items {
			label := fmt.Sprintf("%s %s / %s / %s",
				vc.ID, vc.Name, vc.Employer.Name, vc.AlternateURL,
			)

			if assessment := assessments[vc.ID]; assessment != nil {
				meta := make([]string, 0, 2)
				if assessment.Score > 0 {
					meta = append(meta, fmt.Sprintf("AI score %.2f", assessment.Score))
				}
				if assessment.Reason != "" {
					reason := assessment.Reason
					if len(reason) > 80 {
						reason = reason[:77] + "..."
					}
					meta = append(meta, reason)
				}
				if len(meta) > 0 {
					label = fmt.Sprintf("%s [%s]", label, strings.Join(meta, " | "))
				}
			}

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

			if err = apply(hh, *logger, resume, &headhunter.Vacancies{Items: v}, config.Apply.Message, assessments); err != nil {
				return err
			}

			vacancies.Exclude(headhunter.VacancyIDField, []string{vacancyID})
		}
	}
}

func apply(hh *headhunter.Client, logger zap.Logger, resume *headhunter.Resume, vacancies *headhunter.Vacancies, defaultMessage string, assessments map[string]*ai.FitAssessment) error {
	if resume == nil {
		return fmt.Errorf("resume is required")
	}

	for _, vacancy := range vacancies.Items {
		if vacancy == nil {
			continue
		}

		message := defaultMessage

		if assessment := assessments[vacancy.ID]; assessment != nil {
			if assessment.Message != "" {
				message = assessment.Message
			} else if message == "" && assessment.Reason != "" {
				message = assessment.Reason
			}
		}

		if message == "" {
			message = defaultFallbackMessage
			logger.Warn("falling back to default message", zap.String("vacancy_id", vacancy.ID))
		}

		if err := hh.ApplyWithMessage(resume, vacancy, message); err != nil {
			return err
		}

		if assessment := assessments[vacancy.ID]; assessment != nil {
			logger.Info("successfully applied to vacancy",
				zap.String("vacancy_id", vacancy.ID),
				zap.String("vacancy_name", vacancy.Name),
				zap.Float64("ai_score", assessment.Score),
			)
		} else {
			logger.Info("successfully applied to vacancy",
				zap.String("vacancy_id", vacancy.ID),
				zap.String("vacancy_name", vacancy.Name),
			)
		}
	}

	logger.Info("successfully applied to vacancies", zap.Int("count", vacancies.Len()))

	return nil
}

func prepareAIEvaluation(ctx context.Context, logger *zap.Logger, cfg *Config, hh *headhunter.Client, resume *headhunter.Resume, vacancies *headhunter.Vacancies) (map[string]*ai.FitAssessment, error) {
	matcher, err := newAIMatcher(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	if matcher == nil {
		return nil, nil
	}
	if resume == nil {
		return nil, fmt.Errorf("resume is required for AI evaluation")
	}

	resumeDetails, err := hh.GetResumeDetails(resume.ID)
	if err != nil {
		return nil, fmt.Errorf("get resume details: %w", err)
	}

	return evaluateVacanciesWithMatcher(ctx, logger, matcher, resumeDetails, hh, vacancies)
}

func newAIMatcher(ctx context.Context, cfg *Config, logger *zap.Logger) (ai.Matcher, error) {
	if cfg == nil || cfg.AI == nil || !cfg.AI.Enabled {
		return nil, nil
	}

	provider := strings.TrimSpace(strings.ToLower(cfg.AI.Provider))
	if provider != "" && provider != "gemini" {
		return nil, fmt.Errorf("unsupported ai provider: %s", cfg.AI.Provider)
	}

	if cfg.AI.Gemini == nil {
		return nil, fmt.Errorf("gemini configuration is required when ai is enabled")
	}

	apiKey := strings.TrimSpace(cfg.AI.Gemini.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	if apiKey == "" {
		return nil, fmt.Errorf("gemini api key is required (set ai.gemini.api-key or GOOGLE_API_KEY/GEMINI_API_KEY)")
	}

	generator, err := gemini.NewGenerator(ctx, apiKey, cfg.AI.Gemini.Model, cfg.AI.Gemini.MaxRetries, logger)
	if err != nil {
		return nil, err
	}

	minScore := cfg.AI.MinimumFitScore
	if minScore < 0 {
		minScore = 0
	}

	logger.Info("AI assistance enabled",
		zap.String("provider", "gemini"),
		zap.Float64("minimum_fit_score", minScore),
		zap.Int("ai_retry_attempts", generator.MaxRetries()),
	)

	matcher := gemini.NewMatcher(generator, logger, minScore)

	return matcher, nil
}

func evaluateVacanciesWithMatcher(ctx context.Context, logger *zap.Logger, matcher ai.Matcher, resumeDetails *headhunter.ResumeDetails, hh *headhunter.Client, vacancies *headhunter.Vacancies) (map[string]*ai.FitAssessment, error) {
	if matcher == nil {
		return nil, nil
	}

	initial := vacancies.Len()
	approved := make([]*headhunter.Vacancy, 0, initial)
	assessments := make(map[string]*ai.FitAssessment)

	for _, vacancy := range vacancies.Items {

		detailed := vacancy
		if full, err := hh.GetVacancy(vacancy.ID); err == nil && full != nil {
			detailed = full
		} else if err != nil {
			logger.Debug("fetching detailed vacancy failed",
				zap.String("vacancy_id", vacancy.ID),
				zap.Error(err),
			)
		}

		assessment, err := matcher.Evaluate(ctx, resumeDetails, detailed)
		if err != nil {
			logger.Warn("AI evaluation failed",
				zap.String("vacancy_id", vacancy.ID),
				zap.Error(err),
			)
			detailed.AI = &headhunter.AIAssessment{Error: err.Error()}
			approved = append(approved, detailed)
			continue
		}

		if !assessment.Fit {
			logger.Info("vacancy rejected by AI provider",
				zap.String("vacancy_id", vacancy.ID),
				zap.Float64("ai_score", assessment.Score),
				zap.String("reason", assessment.Reason),
			)
			continue
		}

		logger.Info("vacancy approved by AI",
			zap.String("vacancy_id", vacancy.ID),
			zap.Float64("ai_score", assessment.Score),
		)

		detailed.AI = &headhunter.AIAssessment{
			Fit:     assessment.Fit,
			Score:   assessment.Score,
			Reason:  assessment.Reason,
			Message: assessment.Message,
			Raw:     assessment.Raw,
		}
		approved = append(approved, detailed)
		assessments[detailed.ID] = assessment
	}

	vacancies.Items = approved

	if initial != len(approved) {
		logger.Info("AI filtering completed",
			zap.Int("initial_vacancies", initial),
			zap.Int("approved_vacancies", len(approved)),
		)
	}

	return assessments, nil
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
