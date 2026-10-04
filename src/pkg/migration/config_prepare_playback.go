package migration

import "gopkg.in/yaml.v3"

// ApplyPreparePlayback 保持旧用户关闭，只有显式配置才准备新录播。
func ApplyPreparePlayback(raw []byte, enabled *bool) error {
	var configured struct {
		OnRecordFinished struct {
			Enabled *bool `yaml:"prepare_playback"`
		} `yaml:"on_record_finished"`
	}
	if err := yaml.Unmarshal(raw, &configured); err != nil {
		return err
	}
	*enabled = false
	if configured.OnRecordFinished.Enabled != nil {
		*enabled = *configured.OnRecordFinished.Enabled
	}
	return nil
}
