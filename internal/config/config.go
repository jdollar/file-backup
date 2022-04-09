package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"

	"github.com/spf13/viper"
)

type BoxConfiguration struct {
  BackupFolderName string `mapstructure:"backup_folder_name" yaml:"backup_folder_name"`
  ClientID string `mapstructure:"client_id" yaml:"client_id"`
  ClientSecret string `mapstructure:"client_secret" yaml:"client_secret"`
  SubjectType string `mapstructure:"subject_type" yaml:"subject_type"`
  SubjectId string `mapstructure:"subject_id" yaml:"subject_id"`
}

type Configuration struct {
  BackupLimit int64 `mapstructure:"backup_limit" yaml:"backup_limit"`
  Box BoxConfiguration `mapstructure:"box" yaml:"box"`
}

func initializeConfig(configDir string) error {
  log.Println("Creating new config file")
  err := os.MkdirAll(configDir, os.ModePerm)
  if err != nil {
    return err
  }

  defaultConfig := Configuration{
    BackupLimit: 50,
    Box: BoxConfiguration{
      BackupFolderName: "minecraftBackups",
    },
  }

  // Convert empty config into bytes and upload it into
  // the runtime viper instance
  defaultBytes, err := yaml.Marshal(defaultConfig)
  if err != nil {
    return err
  }
  viper.ReadConfig(bytes.NewBuffer(defaultBytes))

  // Create the config file with the config stored in the
  // runtime viper instance
  return viper.SafeWriteConfig()
}

func NewConfiguration() (Configuration, error) {
  var config Configuration

  homeDir, err := os.UserHomeDir()
  if err != nil {
    return config, err
  }

  configDir := filepath.Join(homeDir, ".dropbox-backup")

  viper.SetConfigName("config")
  viper.SetConfigType("yaml")
  viper.AddConfigPath(configDir)

  if err := viper.ReadInConfig(); err != nil {
    if _, ok := err.(viper.ConfigFileNotFoundError); ok {
      err := initializeConfig(configDir)
      if err != nil {
        return config, err
      }
    } else {
      return config, err
    }
  }

  err = viper.Unmarshal(&config)
  return config, err
}
