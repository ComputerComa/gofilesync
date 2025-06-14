package main

import (
	"fmt"
	"os"

	"github.com/kardianos/service"
)

type program struct {
	configPath string
}

func (p *program) Start(s service.Service) error {
	go p.run()
	return nil
}

func (p *program) run() {
	cfg, err := loadConfig(p.configPath)
	if err != nil {
		LogError("Service failed to load config: %v", err)
		return
	}
	if err := InitLogger(cfg.LogLevel, cfg.LogFile); err != nil {
		fmt.Println("Service failed to init logger:", err)
		return
	}
	defer CloseLogger()
	LogInfo("Service started, watching and syncing...")
	watchAndSync(cfg)
}

func (p *program) Stop(s service.Service) error {
	LogInfo("Service stopping...")
	return nil
}

func runServiceCommand(action, configPath string) error {
	svcConfig := &service.Config{
		Name:        "gofilesync",
		DisplayName: "GoFileSync",
		Description: "Folder-to-SFTP sync service.",
	}
	prg := &program{configPath: configPath}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		return err
	}
	switch action {
	case "install":
		return s.Install()
	case "uninstall":
		return s.Uninstall()
	case "start":
		return s.Start()
	case "stop":
		return s.Stop()
	case "run":
		return s.Run()
	default:
		return fmt.Errorf("unknown service action: %s", action)
	}
}

var _ = os.Stderr // Remove unused import warning
