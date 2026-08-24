package migration

import "gopkg.in/yaml.v3"

// ApplyHLSCacheConfigV210 为 v2.1.0 之前的配置补齐 HLS 缓存默认值。
// 指针字段用于区分“未配置”和用户显式写入的 0，后者表示关闭对应限制。
func ApplyHLSCacheConfigV210(
	raw []byte,
	maxAgeHours *int,
	maxTotalSizeGB *float64,
	defaultMaxAgeHours int,
	defaultMaxTotalSizeGB float64,
) error {
	var configured struct {
		HLSCache *struct {
			MaxAgeHours    *int     `yaml:"max_age_hours"`
			MaxTotalSizeGB *float64 `yaml:"max_total_size_gb"`
		} `yaml:"hls_cache"`
	}
	if err := yaml.Unmarshal(raw, &configured); err != nil {
		return err
	}

	if configured.HLSCache == nil || configured.HLSCache.MaxAgeHours == nil {
		*maxAgeHours = defaultMaxAgeHours
	} else {
		*maxAgeHours = *configured.HLSCache.MaxAgeHours
	}
	if configured.HLSCache == nil || configured.HLSCache.MaxTotalSizeGB == nil {
		*maxTotalSizeGB = defaultMaxTotalSizeGB
	} else {
		*maxTotalSizeGB = *configured.HLSCache.MaxTotalSizeGB
	}
	return nil
}
