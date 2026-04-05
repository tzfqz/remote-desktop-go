package config

import (
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig 加载配置文件
func LoadConfig(filePath string, config interface{}) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", filePath)
	}

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read config file error: %v", err)
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return fmt.Errorf("parse config file error: %v", err)
	}

	return nil
}
