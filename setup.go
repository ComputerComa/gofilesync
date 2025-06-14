package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/zalando/go-keyring"
)

type Config struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	RemotePath string `json:"remote_path"`
	LocalPath  string `json:"local_path"`
	UseKeyring bool   `json:"use_keyring"`
	Password   string `json:"password,omitempty"`
	LogLevel   string `json:"log_level,omitempty"`
	LogFile    string `json:"log_file,omitempty"`
}

func interactiveSetup(configPath string) error {
	fmt.Println("--- GoFileSync Setup ---")
	cfg := Config{}
	_ = survey.AskOne(&survey.Input{Message: "SFTP Host:"}, &cfg.Host, survey.WithValidator(survey.Required))
	portStr := "22"
	_ = survey.AskOne(&survey.Input{Message: "SFTP Port:", Default: "22"}, &portStr, survey.WithValidator(survey.Required))
	fmt.Sscanf(portStr, "%d", &cfg.Port)
	_ = survey.AskOne(&survey.Input{Message: "SFTP Username:"}, &cfg.Username, survey.WithValidator(survey.Required))
	_ = survey.AskOne(&survey.Input{Message: "Remote SFTP Path:", Default: "/"}, &cfg.RemotePath, survey.WithValidator(survey.Required))
	_ = survey.AskOne(&survey.Input{Message: "Local folder to watch:", Default: "."}, &cfg.LocalPath, survey.WithValidator(survey.Required))
	useKeyring := false
	_ = survey.AskOne(&survey.Confirm{Message: "Store password in OS keyring?", Default: true}, &useKeyring)
	cfg.UseKeyring = useKeyring
	pw := ""
	_ = survey.AskOne(&survey.Password{Message: "SFTP Password:"}, &pw, survey.WithValidator(survey.Required))
	if useKeyring {
		err := keyring.Set("gofilesync", cfg.Username, pw)
		if err != nil {
			fmt.Println("Failed to store password in keyring:", err)
			cfg.UseKeyring = false
			cfg.Password = pw
		}
	} else {
		cfg.Password = pw
	}
	_ = survey.AskOne(&survey.Select{Message: "Log level:", Options: []string{"info", "debug", "error"}, Default: "info"}, &cfg.LogLevel)
	_ = survey.AskOne(&survey.Input{Message: "Log file path (leave blank for console only):", Default: ""}, &cfg.LogFile)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0600)
	fmt.Println("Config saved to", configPath)
	return nil
}

func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.UseKeyring {
		pw, err := keyring.Get("gofilesync", cfg.Username)
		if err != nil {
			return nil, fmt.Errorf("failed to get password from keyring: %w", err)
		}
		cfg.Password = pw
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	return &cfg, nil
}
