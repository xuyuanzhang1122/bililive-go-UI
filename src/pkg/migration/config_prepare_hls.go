package migration

import (
	"fmt"
	"gopkg.in/yaml.v3"
)

// ApplyPrepareHLS 不为旧配置增加准备档位，只有显式配置才生成持久 HLS。
func ApplyPrepareHLS(raw []byte, profiles *[]string) error {
	var configured struct {
		Finished struct {
			Profiles []string `yaml:"prepare_hls_profiles"`
		} `yaml:"on_record_finished"`
	}
	if err := yaml.Unmarshal(raw, &configured); err != nil {
		return err
	}
	if err := ValidateHLSProfiles(configured.Finished.Profiles); err != nil {
		return err
	}
	*profiles = append([]string(nil), configured.Finished.Profiles...)
	return nil
}

func ValidateHLSProfiles(profiles []string) error {
	seen := make(map[string]bool)
	for _, profile := range profiles {
		if (profile != "source" && profile != "720p" && profile != "1080p") || seen[profile] {
			return fmt.Errorf("持久 HLS 档位必须是 source/720p/1080p 且不能重复")
		}
		seen[profile] = true
	}
	return nil
}
