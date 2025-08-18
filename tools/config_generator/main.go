package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// FieldInfo 字段信息
type FieldInfo struct {
	Name     string
	Type     string
	JSONName string
	Comment  string
	Order    int // 字段顺序
}

// ConfigInfo 配置信息
type ConfigInfo struct {
	FileName    string
	StructName  string
	PackageName string
	Fields      []FieldInfo
	SampleData  interface{}
}

// 生成Go代码的模板
const goTemplate = `package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
)

// {{.StructName}} {{.FileName}}配置结构体
type {{.StructName}} struct {
{{range .Fields}}	{{.Name}} {{.Type}} ` + "`json:\"{{.JSONName}}\"`" + ` // {{.Comment}}
{{end}}}

// {{.StructName}}Cache {{.FileName}}配置缓存
type {{.StructName}}Cache struct {
	cache map[string]*{{.StructName}}
	mu    sync.RWMutex
}

var {{.StructName}}CacheInstance = &{{.StructName}}Cache{
	cache: make(map[string]*{{.StructName}}),
}

// get{{.StructName}}FromCache 从缓存获取配置
func (c *{{.StructName}}Cache) get{{.StructName}}FromCache(id string) (*{{.StructName}}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// set{{.StructName}}ToCache 设置配置到缓存
func (c *{{.StructName}}Cache) set{{.StructName}}ToCache(id string, item *{{.StructName}}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clear{{.StructName}}Cache 清空缓存
func (c *{{.StructName}}Cache) clear{{.StructName}}Cache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[string]*{{.StructName}})
}

// convertTo{{.StructName}} 将原始配置转换为{{.StructName}}结构体
func convertTo{{.StructName}}(config interface{}) (*{{.StructName}}, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &{{.StructName}}{}
		
		// 使用反射设置字段值
		configValue := reflect.ValueOf(result).Elem()
		configType := configValue.Type()
		
		for i := 0; i < configValue.NumField(); i++ {
			field := configValue.Field(i)
			fieldType := configType.Field(i)
			jsonTag := fieldType.Tag.Get("json")
			
			if value, exists := configMap[jsonTag]; exists {
				// 根据字段类型进行类型转换
				switch field.Kind() {
				case reflect.String:
					if str, ok := value.(string); ok {
						field.SetString(str)
					}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					if num, ok := value.(float64); ok {
						field.SetInt(int64(num))
					}
				case reflect.Float32, reflect.Float64:
					if num, ok := value.(float64); ok {
						field.SetFloat(num)
					}
				case reflect.Bool:
					if b, ok := value.(bool); ok {
						field.SetBool(b)
					}
				case reflect.Slice:
					if slice, ok := value.([]interface{}); ok {
						// 处理字符串切片
						if field.Type().Elem().Kind() == reflect.String {
							strSlice := make([]string, len(slice))
							for j, item := range slice {
								if str, ok := item.(string); ok {
									strSlice[j] = str
								}
							}
							field.Set(reflect.ValueOf(strSlice))
						}
					}
				}
			}
		}
		
		return result, true
	}

	return nil, false
}

// Get{{.StructName}}Config 获取{{.FileName}}配置（带缓存）
func Get{{.StructName}}Config(id string) (*{{.StructName}}, bool) {
	// 先从缓存获取
	if item, exists := {{.StructName}}CacheInstance.get{{.StructName}}FromCache(id); exists {
		return item, true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("{{.FileName}}", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if item, ok := convertTo{{.StructName}}(config); ok {
		// 设置到缓存
		{{.StructName}}CacheInstance.set{{.StructName}}ToCache(id, item)
		return item, true
	}

	return nil, false
}

// GetAll{{.StructName}}Configs 获取所有{{.FileName}}配置（带缓存）
func GetAll{{.StructName}}Configs() (map[string]*{{.StructName}}, bool) {
	configs, exists := config.GetAllConfigs("{{.FileName}}")
	if !exists {
		return nil, false
	}

	result := make(map[string]*{{.StructName}})
	for id := range configs {
		if item, ok := Get{{.StructName}}Config(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// Get{{.StructName}}Name 获取{{.FileName}}名称
func Get{{.StructName}}Name(id string) (string, bool) {
	if item, exists := Get{{.StructName}}Config(id); exists {
		return item.Name, true
	}
	return "", false
}

// Reload{{.StructName}}Config 重新加载{{.FileName}}配置并清空缓存
func Reload{{.StructName}}Config() error {
	// 清空缓存
	{{.StructName}}CacheInstance.clear{{.StructName}}Cache()
	
	// 重新加载配置
	return config.ReloadConfig("{{.FileName}}")
}

// Validate{{.StructName}}Config 验证{{.FileName}}配置
func Validate{{.StructName}}Config(id string) error {
	if _, exists := Get{{.StructName}}Config(id); !exists {
		return fmt.Errorf("配置不存在: %s", id)
	}
	return nil
}

// Clear{{.StructName}}Cache 手动清空{{.FileName}}配置缓存
func Clear{{.StructName}}Cache() {
	{{.StructName}}CacheInstance.clear{{.StructName}}Cache()
}
`

func main() {
	// 从配置文件读取路径配置
	configDir, outputDir, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	// 扫描JSON文件
	files, err := filepath.Glob(filepath.Join(configDir, "*.json"))
	if err != nil {
		log.Fatalf("扫描配置文件失败: %v", err)
	}

	fmt.Printf("发现 %d 个JSON配置文件\n", len(files))
	fmt.Printf("配置目录: %s\n", configDir)
	fmt.Printf("输出目录: %s\n", outputDir)

	// 按文件名排序
	sort.Strings(files)

	var configInfos []*ConfigInfo

	for _, file := range files {
		fileName := filepath.Base(file)
		fmt.Printf("处理文件: %s\n", fileName)

		// 解析JSON文件
		configInfo, err := parseJSONFile(file, fileName)
		if err != nil {
			log.Printf("解析文件 %s 失败: %v", fileName, err)
			continue
		}

		// 生成Go代码
		if err := generateGoFile(configInfo, outputDir); err != nil {
			log.Printf("生成Go文件失败 %s: %v", fileName, err)
			continue
		}

		configInfos = append(configInfos, configInfo)
		fmt.Printf("成功生成: %s.go\n", configInfo.StructName)
	}

	// 生成注册文件
	if err := generateRegisterFile(configInfos, outputDir); err != nil {
		log.Printf("生成注册文件失败: %v", err)
	} else {
		fmt.Println("成功生成: register.go")
	}

	fmt.Println("配置生成完成！")
	fmt.Printf("输出目录: %s\n", outputDir)
}

// loadConfig 从配置文件加载路径配置
func loadConfig() (string, string, error) {
	// 默认配置
	defaultConfig := map[string]string{
		"configDir": "../../conf/config",
		"outputDir": "../../common/config/generated",
	}

	// 尝试读取配置文件
	configFile := "config_generator.conf"
	if data, err := ioutil.ReadFile(configFile); err == nil {
		// 解析配置文件
		var config map[string]string
		if err := json.Unmarshal(data, &config); err == nil {
			// 使用配置文件中的值，如果没有则使用默认值
			configDir := config["configDir"]
			if configDir == "" {
				configDir = defaultConfig["configDir"]
			}

			outputDir := config["outputDir"]
			if outputDir == "" {
				outputDir = defaultConfig["outputDir"]
			}

			return configDir, outputDir, nil
		}
	}

	// 如果配置文件不存在或解析失败，使用默认配置
	fmt.Printf("使用默认配置:\n")
	fmt.Printf("  配置目录: %s\n", defaultConfig["configDir"])
	fmt.Printf("  输出目录: %s\n", defaultConfig["outputDir"])
	fmt.Printf("要自定义路径，请创建 %s 文件，格式如下:\n", configFile)
	fmt.Printf("{\n")
	fmt.Printf("  \"configDir\": \"你的配置目录路径\",\n")
	fmt.Printf("  \"outputDir\": \"你的输出目录路径\"\n")
	fmt.Printf("}\n\n")

	return defaultConfig["configDir"], defaultConfig["outputDir"], nil
}

// parseJSONFile 解析JSON文件并提取结构信息
func parseJSONFile(filePath, fileName string) (*ConfigInfo, error) {
	// 读取文件内容
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// 解析JSON
	var jsonData interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, err
	}

	// 提取字段信息
	fields := extractFields(jsonData)

	// 生成结构体名称
	structName := generateStructName(fileName)

	return &ConfigInfo{
		FileName:    fileName,
		StructName:  structName,
		PackageName: "config",
		Fields:      fields,
		SampleData:  jsonData,
	}, nil
}

// extractFields 从JSON数据中提取字段信息
func extractFields(data interface{}) []FieldInfo {
	var fields []FieldInfo

	switch v := data.(type) {
	case []interface{}:
		if len(v) > 0 {
			if mapData, ok := v[0].(map[string]interface{}); ok {
				fields = extractFieldsFromMap(mapData)
			}
		}
	case map[string]interface{}:
		fields = extractFieldsFromMap(v)
	}

	// 按字段顺序排序
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Order < fields[j].Order
	})

	return fields
}

// extractFieldsFromMap 从map中提取字段信息
func extractFieldsFromMap(data map[string]interface{}) []FieldInfo {
	var fields []FieldInfo
	order := 0

	// 定义字段优先级顺序
	priorityFields := []string{"id", "name", "type", "level", "hp", "attack", "defense", "exp", "price", "durability", "description"}

	// 按优先级处理字段
	for _, priorityKey := range priorityFields {
		if value, exists := data[priorityKey]; exists {
			fields = append(fields, FieldInfo{
				Name:     toCamelCase(priorityKey),
				JSONName: priorityKey,
				Type:     inferGoType(value),
				Comment:  priorityKey,
				Order:    order,
			})
			order++
		}
	}

	// 处理剩余的字段
	for key, value := range data {
		// 检查是否已经处理过
		alreadyProcessed := false
		for _, priorityKey := range priorityFields {
			if key == priorityKey {
				alreadyProcessed = true
				break
			}
		}

		if !alreadyProcessed {
			field := FieldInfo{
				Name:     toCamelCase(key),
				JSONName: key,
				Type:     inferGoType(value),
				Comment:  key,
				Order:    order,
			}
			fields = append(fields, field)
			order++
		}
	}

	return fields
}

// inferGoType 推断Go类型
func inferGoType(value interface{}) string {
	switch v := value.(type) {
	case string:
		return "string"
	case float64:
		// 对于游戏配置，我们倾向于使用float64来保持精度
		// 特别是对于时间、距离等可能需要小数的值
		return "float64"
	case bool:
		return "bool"
	case []interface{}:
		if len(v) > 0 {
			// 检查切片元素类型
			switch v[0].(type) {
			case string:
				return "[]string"
			case float64:
				return "[]float64"
			default:
				return "[]interface{}"
			}
		}
		return "[]interface{}"
	default:
		return "interface{}"
	}
}

// toCamelCase 转换为驼峰命名
func toCamelCase(s string) string {
	// 处理Go关键字冲突
	if s == "type" {
		return "Type"
	}

	parts := strings.Split(s, "_")
	for i, part := range parts {
		if i > 0 && len(part) > 0 {
			parts[i] = strings.Title(part)
		}
	}
	return strings.Title(parts[0])
}

// generateStructName 生成结构体名称
func generateStructName(fileName string) string {
	// 移除.json扩展名
	name := strings.TrimSuffix(fileName, ".json")

	// 转换为单数形式并首字母大写
	if strings.HasSuffix(name, "s") {
		name = name[:len(name)-1]
	}

	return strings.Title(name)
}

// generateGoFile 生成Go文件
func generateGoFile(configInfo *ConfigInfo, outputDir string) error {
	// 创建模板
	tmpl, err := template.New("config").Parse(goTemplate)
	if err != nil {
		return err
	}

	// 创建输出文件
	outputFile := filepath.Join(outputDir, strings.ToLower(configInfo.StructName)+".go")
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// 执行模板
	return tmpl.Execute(file, configInfo)
}

// generateRegisterFile 生成注册文件
func generateRegisterFile(configInfos []*ConfigInfo, outputDir string) error {
	// 注册文件模板
	const registerTemplate = `package config

import (
	"gameserver/common/config"
)

// init 函数在包初始化时自动执行
func init() {
	// 注册所有生成的配置 reload 函数
	// 这样当调用 config.ReloadAll() 时，会自动调用这些函数
	
{{range .}}	// 注册{{.StructName}}配置重载函数
	config.RegisterReloadFunc(func() error {
		return Reload{{.StructName}}Config()
	})
{{end}}
}
`

	// 创建模板
	tmpl, err := template.New("register").Parse(registerTemplate)
	if err != nil {
		return err
	}

	// 创建输出文件
	outputFile := filepath.Join(outputDir, "register.go")
	file, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// 执行模板
	return tmpl.Execute(file, configInfos)
}
