package playbackasset

// Windows 明确使用较弱的目录持久性：文件内容仍由各发布入口 Sync，
// 命名空间仍通过不可覆盖硬链接发布；此处验证父目录访问和类型，不调用
// Unix 式目录 Sync，也不把任何访问错误当作“不支持”吞掉。
// 成功不承诺断电后目录项持久化，需在真实 Windows 文件系统另行验收。
func syncPublicationDir(path string) error {
	return syncPublicationParent(path, false)
}
