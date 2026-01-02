package main

import (
	"bufio"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// FileEntry 存储文件元数据
type FileEntry struct {
	Name string
	Path string
	Size int64
}

func main() {
	// 1. 解析命令行参数
	dirPtr := flag.String("dir", ".", "指定要遍历的根目录路径")
	flag.Parse()

	rootDir := *dirPtr

	// 验证目录是否存在
	info, err := os.Stat(rootDir)
	if err != nil || !info.IsDir() {
		fmt.Printf("错误: 目录 '%s' 不存在或不是一个目录\n", rootDir)
		os.Exit(1)
	}

	fmt.Println("--------------------------------------------------")
	fmt.Printf("正在扫描目录: %s\n", rootDir)
	fmt.Println("请稍候，正在计算哈希并比对同一层级文件...")
	fmt.Println("--------------------------------------------------")

	// 用于收集所有待删除的文件路径
	var filesToDelete []string

	// 2. 开始递归扫描，传入切片指针以收集数据
	processDirectory(rootDir, &filesToDelete)

	// 3. 扫描结束，检查结果
	if len(filesToDelete) == 0 {
		fmt.Println("\n太棒了！没有发现同一层级的重复文件。")
		return
	}

	// 4. 列出待删除文件清单
	fmt.Printf("\n--------------------------------------------------\n")
	fmt.Printf("扫描完成！共发现 %d 个重复文件待清理：\n", len(filesToDelete))
	fmt.Printf("--------------------------------------------------\n")

	for i, path := range filesToDelete {
		fmt.Printf("[%d] %s\n", i+1, path)
	}

	// 5. 交互式确认
	fmt.Printf("\n警告: 以上文件将被永久删除且无法恢复。\n")
	fmt.Print("是否确认删除？请输入 (y/n): ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "y" {
		fmt.Println("\n正在删除...")
		performDeletion(filesToDelete)
		fmt.Println("--------------------------------------------------")
		fmt.Println("清理完成。")
	} else {
		fmt.Println("\n操作已取消，未删除任何文件。")
	}
}

// performDeletion 批量执行删除
func performDeletion(paths []string) {
	successCount := 0
	failCount := 0

	for _, path := range paths {
		err := os.Remove(path)
		if err != nil {
			fmt.Printf("[删除失败] %s: %v\n", path, err)
			failCount++
		} else {
			fmt.Printf("[已删除] %s\n", path)
			successCount++
		}
	}
	fmt.Printf("\n统计: 成功删除 %d 个, 失败 %d 个\n", successCount, failCount)
}

// processDirectory 递归处理目录
// 注意：这里传入的是 *[]string 指针，以便在递归中追加数据
func processDirectory(dirPath string, toDelete *[]string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		fmt.Printf("无法读取目录 %s: %v\n", dirPath, err)
		return
	}

	var subDirs []string
	filesBySize := make(map[int64][]FileEntry)

	// 分类：收集子目录，并将文件按大小分组
	for _, entry := range entries {
		fullPath := filepath.Join(dirPath, entry.Name())

		if entry.IsDir() {
			subDirs = append(subDirs, fullPath)
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		size := info.Size()
		filesBySize[size] = append(filesBySize[size], FileEntry{
			Name: entry.Name(),
			Path: fullPath,
			Size: size,
		})
	}

	// 处理当前目录下的重复文件
	detectAndCollect(filesBySize, toDelete)

	// 递归处理子目录
	for _, subDir := range subDirs {
		processDirectory(subDir, toDelete)
	}
}

// detectAndCollect 检测哈希并将待删除文件加入列表
func detectAndCollect(filesBySize map[int64][]FileEntry, toDelete *[]string) {
	for _, entries := range filesBySize {
		if len(entries) < 2 {
			continue
		}

		filesByHash := make(map[string][]FileEntry)

		for _, entry := range entries {
			hash, err := calculateFileHash(entry.Path)
			if err != nil {
				fmt.Printf("计算哈希失败 %s: %v\n", entry.Path, err)
				continue
			}
			filesByHash[hash] = append(filesByHash[hash], entry)
		}

		for hash, duplicates := range filesByHash {
			if len(duplicates) > 1 {
				// 发现重复，筛选出要删除的
				recordDuplicates(duplicates, hash, toDelete)
			}
		}
	}
}

// recordDuplicates 决定保留哪个，将其余的加入待删除列表
func recordDuplicates(files []FileEntry, hash string, toDelete *[]string) {
	// 排序逻辑：名字短的排前面，长度一样按字母序
	slices.SortFunc(files, func(a, b FileEntry) int {
		// 1. 优先比较文件名长度
		if n := cmp.Compare(len(a.Name), len(b.Name)); n != 0 {
			return n
		}
		// 2. 长度相同时，比较文件名字母序
		return cmp.Compare(a.Name, b.Name)
	})

	keep := files[0]
	discardCandidates := files[1:]

	// 打印实时的发现日志（可选，为了让用户知道进度）
	fmt.Printf("发现重复 (Hash: %s...): 保留 [%s]\n", hash[:8], keep.Name)

	// 将要删除的文件路径加入总列表
	for _, f := range discardCandidates {
		*toDelete = append(*toDelete, f.Path)
	}
}

// calculateFileHash 计算文件的 SHA256 哈希
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
