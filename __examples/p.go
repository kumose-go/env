package main

import (
	"fmt"
	"log"
	"time"

	"yourmodule/env" // 替换成你的 env 包路径
)

func main() {
	manager := &env.EnvManager{}

	// 模拟三个 fragment，分别是系统、内部组件、自定义
	systemFrag := &env.EnvFragment{
		Name:     "system_base",
		Priority: 10,
		Env: map[string]string{
			"PATH": "/usr/local/bin:/usr/bin",
			"LANG": "en_US.UTF-8",
		},
		Script: []env.Script{
			{
				Sh:   "bash",
				Data: `echo "System base script executed"`},
			{
				Sh:   "pwsh",
				Data: `Write-Host "System base PowerShell script"`},
		},
		Source: "system.yaml",
	}

	innerFrag := &env.EnvFragment{
		Name:     "internal_service",
		Priority: 30,
		Env: map[string]string{
			"LANG":     "zh_CN.UTF-8", // 覆盖系统的 LANG
			"APP_HOME": "/opt/app",
		},
		Script: []env.Script{
			{
				Sh: "bash",
				Data: `if [ -z "$APP_URL" ]; then
  export APP_URL="http://localhost:8080"
fi`},
			{
				Sh: "zsh",
				Data: `if [[ -z "$APP_URL" ]]; then
  export APP_URL="http://localhost:8080"
fi`},
		},
		Source: "internal.yaml",
	}

	customFrag := &env.EnvFragment{
		Name:     "user_service",
		Priority: 150,
		Env: map[string]string{
			"APP_HOME": "/home/user/app", // 覆盖内部组件
			"DEBUG":    "true",
		},
		Script: []env.Script{
			{
				Sh:   "bash",
				Data: `echo "User service Bash script"`},
			{
				Sh:   "pwsh",
				Data: `Write-Host "User service PowerShell script"`},
		},
		Source: "user.yaml",
	}

	// 添加 fragment
	manager.fragments = append(manager.fragments, systemFrag, innerFrag, customFrag)

	// 排序合并
	manager.SortAndMerge()

	// 输出 Bash env 文件
	bashFile := "env_test.sh"
	if err := manager.BuildBash(bashFile); err != nil {
		log.Fatalf("BuildBash error: %v", err)
	}

	// 输出 Zsh env 文件
	zshFile := "env_test.zsh"
	if err := manager.BuildZsh(zshFile); err != nil {
		log.Fatalf("BuildZsh error: %v", err)
	}

	// 输出 PowerShell env 文件
	psFile := "env_test.ps1"
	if err := manager.BuildPsh(psFile); err != nil {
		log.Fatalf("BuildPsh error: %v", err)
	}

	// 写 meta 文件
	metaFile := "env_test.meta"
	if err := manager.WriteMeta(metaFile); err != nil {
		log.Fatalf("WriteMeta error: %v", err)
	}

	// 读取 meta 文件
	t, err := env.ReadEnvTime(metaFile)
	if err != nil {
		log.Fatalf("ReadEnvTime error: %v", err)
	}
	fmt.Printf("Env generated at: %s\n", t.Format(time.RFC3339))

	// 搜索示例
	results, err := manager.Search("APP_HOME")
	if err != nil {
		log.Fatalf("Search error: %v", err)
	}
	for _, r := range results {
		fmt.Printf("Found: fragment=%s, key=%s, value=%s\n", r.FragmentName, r.Key, r.Value)
	}

	fmt.Println("Env files generated successfully:")
	fmt.Println(" -", bashFile)
	fmt.Println(" -", zshFile)
	fmt.Println(" -", psFile)
	fmt.Println(" - meta:", metaFile)
}
