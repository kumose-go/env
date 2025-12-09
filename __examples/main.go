package main

import (
	"fmt"
	"log"
	"time"

	"github.com/kumose-go/env"
)

func main() {
	// 初始化 EnvManager
	em := &env.EnvManager{}

	// 1. 加载目录下所有 YAML 文件
	if err := em.FeedDir("./env_fragments"); err != nil {
		log.Fatalf("FeedDir error: %v", err)
	}

	// 2. 排序并合并
	em.SortAndMerge()

	// 3. 生成各 shell 环境文件
	if err := em.BuildBash("./env_generated.sh"); err != nil {
		log.Fatalf("BuildBash error: %v", err)
	}
	if err := em.BuildZsh("./env_generated.zsh"); err != nil {
		log.Fatalf("BuildZsh error: %v", err)
	}
	if err := em.BuildPsh("./env_generated.ps1"); err != nil {
		log.Fatalf("BuildPsh error: %v", err)
	}

	// 4. 写 meta 时间文件
	if err := em.WriteMeta("./env_generated.meta"); err != nil {
		log.Fatalf("WriteMeta error: %v", err)
	}

	// 5. 读取时间并判断是否过期
	t, err := env.ReadEnvTime("./env_generated.meta")
	if err != nil {
		log.Fatalf("ReadEnvTime error: %v", err)
	}
	fmt.Println("Env generated at:", t.Format(time.RFC3339))
	if time.Since(t) > 24*time.Hour {
		fmt.Println("Warning: Env is older than 24h, consider regenerating")
	}

	// 6. 搜索变量
	results, err := em.Search("SERVICE_PORT")
	if err != nil {
		log.Fatalf("Search error: %v", err)
	}
	fmt.Println("Search results for SERVICE_PORT:")
	for _, r := range results {
		fmt.Printf("Fragment: %s, Key: %s, Value: %s\n", r.FragmentName, r.Key, r.Value)
	}
}
