package configs

// 功能特征标志
// 用于控制正在开发中的功能是否启用
// 开发者可以在本地修改这些值来启用/禁用功能

// EnableProxyConfig 控制是否启用代理配置功能
// false: 隐藏前端代理配置 UI，后端代理函数回退到环境变量
// true: 启用完整的代理配置（通用代理 + 信息获取代理 + 下载代理）
const EnableProxyConfig = false
