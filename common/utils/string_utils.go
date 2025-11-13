package utils

import "path/filepath"

// ExtractAndReplace 从文件路径中提取文件名并替换为新文件名
func ExtractAndReplace(filePath, destFileName string) string {
	// 获取文件扩展名
	ext := filepath.Ext(filePath)

	// 获取目录路径
	dir := filepath.Dir(filePath)

	// 构建新路径
	return dir + "/" + destFileName + ext
}
