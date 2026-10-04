package servers

import (
	"math"
	"time"
)

// RetryAfter 同时反映失败退避和读租约宽限；后续读取会延长实际重试时间。
func (m *hlsJobManager) RetryAfter(j *hlsJob) int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	j.mutex.Lock()
	defer j.mutex.Unlock()
	retryAt := j.finishedAt.Add(hlsRetryDelay)
	readAt := m.lastRead[j.key].Add(j.readGrace())
	if readAt.After(retryAt) {
		retryAt = readAt
	}
	seconds := int(math.Ceil(time.Until(retryAt).Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}
