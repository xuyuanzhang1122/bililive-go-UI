package migration

import "gopkg.in/yaml.v3"

// ApplyHLSResourceConfigV210 为旧配置补齐 HLS 资源限制，保留显式零值供校验或关闭功能。
func ApplyHLSResourceConfigV210(raw []byte,
	maxConcurrent, maxQueued *int, minFreeSpaceGB *float64, readGraceSeconds *int,
	defaultMaxConcurrent, defaultMaxQueued int, defaultMinFreeSpaceGB float64, defaultReadGraceSeconds int,
) error {
	var configured struct {
		HLSCache struct {
			MaxConcurrent    *int     `yaml:"max_concurrent"`
			MaxQueued        *int     `yaml:"max_queued"`
			MinFreeSpaceGB   *float64 `yaml:"min_free_space_gb"`
			ReadGraceSeconds *int     `yaml:"read_grace_seconds"`
		} `yaml:"hls_cache"`
	}
	if err := yaml.Unmarshal(raw, &configured); err != nil {
		return err
	}
	*maxConcurrent = defaultMaxConcurrent
	*maxQueued = defaultMaxQueued
	*minFreeSpaceGB = defaultMinFreeSpaceGB
	*readGraceSeconds = defaultReadGraceSeconds
	if value := configured.HLSCache.MaxConcurrent; value != nil {
		*maxConcurrent = *value
	}
	if value := configured.HLSCache.MaxQueued; value != nil {
		*maxQueued = *value
	}
	if value := configured.HLSCache.MinFreeSpaceGB; value != nil {
		*minFreeSpaceGB = *value
	}
	if value := configured.HLSCache.ReadGraceSeconds; value != nil {
		*readGraceSeconds = *value
	}
	return nil
}
