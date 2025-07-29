package config

import (
    "os"
    "gopkg.in/yaml.v3"
)

type GroveConfig struct {
    // Configuration structure will be defined based on requirements
    Version string `yaml:"version"`
}

func LoadGroveConfig() (*GroveConfig, error) {
    // Search for grove.yml in current directory and parent directories
    configPath, err := findConfigFile()
    if err != nil {
        return nil, err
    }
    
    data, err := os.ReadFile(configPath)
    if err != nil {
        return nil, err
    }
    
    var config GroveConfig
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, err
    }
    
    return &config, nil
}


func findConfigFile() (string, error) {
    // Start from current directory and walk up
    dir, err := os.Getwd()
    if err != nil {
        return "", err
    }
    
    return FindConfigFile(dir)
}