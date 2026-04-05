package config

import (
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig 加载 yaml 配置文件到 target（target 须为指针）
func LoadConfig(filePath string, target interface{}) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", filePath)
	}
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read config error: %v", err)
	}
	return yaml.Unmarshal(data, target)
}
