package build

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/alecthomas/kingpin"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
)

// AI 指示文件路径
// trackedAgentFiles 纳入版本控制，check-agents 在 CI 中校验；
// localAgentFiles 在 .gitignore 中（开发者本地镜像），仅 sync-agents 写入、存在时才校验。
var trackedAgentFiles = []string{
	".github/copilot-instructions.md",
}

var localAgentFiles = []string{
	".agent/rules/gemini-guide.md",
	".gemini/GEMINI.md",
}

const sourceAgentFile = "AGENTS.md"

// 全局变量，用于存储命令行参数
var customVersion string
var customPlatform string
var customArch string
var customNoTelemetry bool

func RunCmd() int {
	app := kingpin.New("Build tool", "bililive-go Build tool.")

	// dev 命令支持 --version 参数
	devCmd := app.Command("dev", "Build for development.")
	devCmd.Flag("version", "自定义版本号（用于测试升级功能）").StringVar(&customVersion)
	devCmd.Flag("platform", "目标平台（如 linux、darwin、windows）").StringVar(&customPlatform)
	devCmd.Flag("arch", "目标架构（如 amd64、arm64）").StringVar(&customArch)
	devCmd.Flag("no-telemetry", "禁用启动统计上报").BoolVar(&customNoTelemetry)
	devCmd.Action(devBuild)

	app.Command("dev-incremental", "增量构建：只在源码变化时重新编译（用于调试）").Action(devIncrementalBuild)

	// release 命令也支持自定义参数（用于本地编译测试版本）
	releaseCmd := app.Command("release", "Build for release.")
	releaseCmd.Flag("version", "自定义版本号").StringVar(&customVersion)
	releaseCmd.Flag("platform", "目标平台（如 linux、darwin、windows）").StringVar(&customPlatform)
	releaseCmd.Flag("arch", "目标架构（如 amd64、arm64）").StringVar(&customArch)
	releaseCmd.Flag("no-telemetry", "禁用启动统计上报").BoolVar(&customNoTelemetry)
	releaseCmd.Action(releaseBuild)
	app.Command("release-docker", "Build for release docker.").Action(releaseDocker)
	app.Command("test", "Run tests.").Action(goTest)
	app.Command("generate", "go generate ./...").Action(goGenerate)
	app.Command("generate-web-api", "根据后端路由生成前端 API 调用表").Action(generateWebAPI)
	app.Command("build-web", "Build webapp.").Action(buildWeb)
	app.Command("sync-agents", "同步 AGENTS.md 到其他 AI 指示文件").Action(syncAgents)
	app.Command("check-agents", "检查 AI 指示文件是否一致").Action(checkAgents)
	app.Command("clean", "清理构建产物").Action(cleanBuild)

	kingpin.MustParse(app.Parse(os.Args[1:]))
	return 0
}

// applyCustomFlags 将命令行参数设置为环境变量供构建函数使用
func applyCustomFlags() {
	if customVersion != "" {
		os.Setenv("APP_VERSION", customVersion)
	}
	if customPlatform != "" {
		os.Setenv("PLATFORM", customPlatform)
	}
	if customArch != "" {
		os.Setenv("ARCH", customArch)
	}
	if customNoTelemetry {
		os.Setenv("NO_TELEMETRY", "1")
	}
}

func devBuild(c *kingpin.ParseContext) error {
	applyCustomFlags()
	BuildGoBinary(true)
	return nil
}

func devIncrementalBuild(c *kingpin.ParseContext) error {
	BuildDevIncremental()
	return nil
}

func releaseBuild(c *kingpin.ParseContext) error {
	applyCustomFlags()
	BuildGoBinary(false)
	return nil
}

func releaseDocker(c *kingpin.ParseContext) error {
	fmt.Printf("release-docker command\n")
	return nil
}

func goTest(c *kingpin.ParseContext) error {
	return utils.ExecCommand([]string{
		"go", "test",
		"-tags", "release",
		"--cover",
		"-coverprofile=coverage.txt",
		"./src/...",
	})
}

func goGenerate(c *kingpin.ParseContext) error {
	return utils.ExecCommand([]string{"go", "generate", "./..."})
}

func buildWeb(c *kingpin.ParseContext) error {
	if err := generateWebAPI(c); err != nil {
		return err
	}
	webappDir := filepath.Join("src", "webapp")
	err := utils.ExecCommandsInDir(
		[][]string{
			{"yarn", "install"},
			{"yarn", "build"},
		},
		webappDir,
	)
	if err != nil {
		return err
	}
	return nil
}

func generateWebAPI(c *kingpin.ParseContext) error {
	cmd := exec.Command("node", filepath.Join("scripts", "generate-web-api.mjs"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// syncAgents 将 AGENTS.md 同步到其他 AI 指示文件
func syncAgents(c *kingpin.ParseContext) error {
	content, err := os.ReadFile(sourceAgentFile)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", sourceAgentFile, err)
	}

	for _, target := range append(append([]string{}, trackedAgentFiles...), localAgentFiles...) {
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("创建目录失败 %s: %w", target, err)
		}
		if err := os.WriteFile(target, content, 0644); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", target, err)
		}
		fmt.Printf("已同步: %s -> %s\n", sourceAgentFile, target)
	}

	fmt.Println("AI 指示文件已同步")
	return nil
}

// checkAgents 检查 AI 指示文件是否一致
func checkAgents(c *kingpin.ParseContext) error {
	source, err := os.ReadFile(sourceAgentFile)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", sourceAgentFile, err)
	}

	allMatch := true
	for _, target := range trackedAgentFiles {
		targetContent, err := os.ReadFile(target)
		if err != nil {
			fmt.Printf("错误：无法读取 %s: %v\n", target, err)
			allMatch = false
			continue
		}

		if !bytes.Equal(source, targetContent) {
			fmt.Printf("错误：%s 与 %s 不一致，请运行 make sync-agents\n", target, sourceAgentFile)
			allMatch = false
		}
	}
	// 本地镜像文件在 .gitignore 中，CI 环境不存在；仅存在时才校验
	for _, target := range localAgentFiles {
		targetContent, err := os.ReadFile(target)
		if err != nil {
			continue
		}
		if !bytes.Equal(source, targetContent) {
			fmt.Printf("错误：%s 与 %s 不一致，请运行 make sync-agents\n", target, sourceAgentFile)
			allMatch = false
		}
	}

	if !allMatch {
		return fmt.Errorf("AI 指示文件不一致")
	}

	fmt.Println("AI 指示文件一致性检查通过")
	return nil
}

// cleanBuild 清理构建产物（跨平台）
func cleanBuild(c *kingpin.ParseContext) error {
	dirsToClean := []string{
		"bin",
		filepath.Join("src", "webapp", "build"),
	}

	for _, dir := range dirsToClean {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("删除 %s 失败: %w", dir, err)
		}
		fmt.Printf("已删除: %s\n", dir)
	}

	fmt.Println("清理完成")
	return nil
}
