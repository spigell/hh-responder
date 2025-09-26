package cmd

import (
	"log"

	"github.com/spigell/hh-responder/internal/headhunter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	app = "hh-responder"
)

type Config struct {
	Search      *headhunter.SearchParams `mapstructure:"search"`
	ExcludeFile string                   `mapstructure:"exclude-file"`
	UserAgent   string                   `mapstructure:"user-agent"`
	TokenFile   string                   `mapstructure:"token-file"`
	Apply       *struct {
		Resume  string
		Message string
		Exclude *struct {
			Employers []string
		}
	}
	AI *AIConfig `mapstructure:"ai"`
}

type AIConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	Provider        string        `mapstructure:"provider"`
	MinimumFitScore float64       `mapstructure:"minimum-fit-score"`
	Gemini          *GeminiConfig `mapstructure:"gemini"`
}

type GeminiConfig struct {
	APIKey       string `mapstructure:"api-key"`
	Model        string `mapstructure:"model"`
	MaxRetries   int    `mapstructure:"max-retries"`
	MaxLogLength int    `mapstructure:"max-log-length"`
}

var (
	// Used for flags.
	cfgFile string

	rootCmd = &cobra.Command{
		Use:   app,
		Short: "hh-responder is a simple cli for searching vacancies on hh.ru and responding to them",
	}
)

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	if err := viper.BindEnv("token-file", "HH_TOKEN_FILE"); err != nil {
		log.Fatalf("binding HH_TOKEN_FILE environment variable: %v", err)
	}

	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "a config file (default is hh-responder.yaml in current directory)")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "verbose/debug output")
	rootCmd.PersistentFlags().BoolP("json", "j", false, "json format for logging")

	viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("json", rootCmd.PersistentFlags().Lookup("json"))
}

func initConfig() {
	// Config needed only for run command now. If there is no config, we can skip initialization
	if runCmd.CalledAs() == "" {
		return
	}

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName(app + ".yaml")
	}

	// We can't proceed if the config file parsed with error.
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal(err)
	}
}

func getConfig() (*Config, error) {
	var config *Config
	err := viper.Unmarshal(&config)
	if err != nil {
		return config, err
	}

	return config, nil
}
