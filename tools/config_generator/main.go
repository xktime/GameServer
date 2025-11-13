package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
	IdType      string // ID字段的类型
}

// 生成Go代码的模板
const goTemplate = `package config

import (
	"fmt"
	"gameserver/common/config"
	"reflect"
	"sync"
	"gameserver/core/log"
)

// {{.StructName}}Data {{.FileName}}配置数据结构体（自动生成，请勿手动修改）
type {{.StructName}}Data struct {
{{range .Fields}}	{{.Name}} {{.Type}} ` + "`json:\"{{.JSONName}}\"`" + ` // {{.Comment}}
{{end}}}

// {{.StructName}} {{.FileName}}配置结构体（可添加自定义方法）
type {{.StructName}} struct {
	*{{.StructName}}Data
}

// New{{.StructName}} 创建新的{{.StructName}}实例
func New{{.StructName}}(data *{{.StructName}}Data) *{{.StructName}} {
	return &{{.StructName}}{
		{{.StructName}}Data: data,
	}
}

// {{.StructName}}Cache {{.FileName}}配置缓存
type {{.StructName}}Cache struct {
	cache map[{{.IdType}}]*{{.StructName}}Data
	mu    sync.RWMutex
}

var {{.StructName}}CacheInstance = &{{.StructName}}Cache{
	cache: make(map[{{.IdType}}]*{{.StructName}}Data),
}

// get{{.StructName}}FromCache 从缓存获取配置
func (c *{{.StructName}}Cache) get{{.StructName}}FromCache(id {{.IdType}}) (*{{.StructName}}Data, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if item, exists := c.cache[id]; exists {
		return item, true
	}
	return nil, false
}

// set{{.StructName}}ToCache 设置配置到缓存
func (c *{{.StructName}}Cache) set{{.StructName}}ToCache(id {{.IdType}}, item *{{.StructName}}Data) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache[id] = item
}

// clear{{.StructName}}Cache 清空缓存
func (c *{{.StructName}}Cache) clear{{.StructName}}Cache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.cache = make(map[{{.IdType}}]*{{.StructName}}Data)
}

// convertTo{{.StructName}} 将原始配置转换为{{.StructName}}Data结构体
func convertTo{{.StructName}}(config interface{}) (*{{.StructName}}Data, bool) {
	if configMap, ok := config.(map[string]interface{}); ok {
		result := &{{.StructName}}Data{}
		
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
						// 处理不同类型的切片
						elemType := field.Type().Elem()
						switch elemType.Kind() {
						case reflect.String:
							strSlice := make([]string, len(slice))
							for j, item := range slice {
								if str, ok := item.(string); ok {
									strSlice[j] = str
								}
							}
							field.Set(reflect.ValueOf(strSlice))
						case reflect.Int32:
							intSlice := make([]int32, len(slice))
							for j, item := range slice {
								if num, ok := item.(float64); ok {
									intSlice[j] = int32(num)
								}
							}
							field.Set(reflect.ValueOf(intSlice))
						case reflect.Float64:
							floatSlice := make([]float64, len(slice))
							for j, item := range slice {
								if num, ok := item.(float64); ok {
									floatSlice[j] = num
								}
							}
							field.Set(reflect.ValueOf(floatSlice))
						case reflect.Bool:
							boolSlice := make([]bool, len(slice))
							for j, item := range slice {
								if b, ok := item.(bool); ok {
									boolSlice[j] = b
								}
							}
							field.Set(reflect.ValueOf(boolSlice))
						case reflect.Slice:
							// 处理二维数组
							if elemType.Elem().Kind() == reflect.String {
								// [][]string
								str2DSlice := make([][]string, len(slice))
								for i, outerItem := range slice {
									if outerSlice, ok := outerItem.([]interface{}); ok {
										strSlice := make([]string, len(outerSlice))
										for j, innerItem := range outerSlice {
											if str, ok := innerItem.(string); ok {
												strSlice[j] = str
											}
										}
										str2DSlice[i] = strSlice
									}
								}
								field.Set(reflect.ValueOf(str2DSlice))
							} else if elemType.Elem().Kind() == reflect.Int32 {
								// [][]int32
								int2DSlice := make([][]int32, len(slice))
								for i, outerItem := range slice {
									if outerSlice, ok := outerItem.([]interface{}); ok {
										intSlice := make([]int32, len(outerSlice))
										for j, innerItem := range outerSlice {
											if num, ok := innerItem.(float64); ok {
												intSlice[j] = int32(num)
											}
										}
										int2DSlice[i] = intSlice
									}
								}
								field.Set(reflect.ValueOf(int2DSlice))
							} else if elemType.Elem().Kind() == reflect.Float64 {
								// [][]float64
								float2DSlice := make([][]float64, len(slice))
								for i, outerItem := range slice {
									if outerSlice, ok := outerItem.([]interface{}); ok {
										floatSlice := make([]float64, len(outerSlice))
										for j, innerItem := range outerSlice {
											if num, ok := innerItem.(float64); ok {
												floatSlice[j] = num
											}
										}
										float2DSlice[i] = floatSlice
									}
								}
								field.Set(reflect.ValueOf(float2DSlice))
							} else {
								// 其他二维数组类型，使用interface{}
								field.Set(reflect.ValueOf(slice))
							}
						default:
							// 对于复杂类型，直接设置为 interface{}
							field.Set(reflect.ValueOf(slice))
						}
					}
				case reflect.Map:
					if mapData, ok := value.(map[string]interface{}); ok {
						// 处理不同类型的map
						keyType := field.Type().Key()
						valueType := field.Type().Elem()
						
						// 检查key类型是否为string
						if keyType.Kind() == reflect.String {
							switch valueType.Kind() {
							case reflect.String:
								// map[string]string
								strMap := make(map[string]string)
								for k, v := range mapData {
									if str, ok := v.(string); ok {
										strMap[k] = str
									}
								}
								field.Set(reflect.ValueOf(strMap))
							case reflect.Int32:
								// map[string]int32
								intMap := make(map[string]int32)
								for k, v := range mapData {
									if num, ok := v.(float64); ok {
										intMap[k] = int32(num)
									}
								}
								field.Set(reflect.ValueOf(intMap))
							case reflect.Float64:
								// map[string]float64
								floatMap := make(map[string]float64)
								for k, v := range mapData {
									if num, ok := v.(float64); ok {
										floatMap[k] = num
									}
								}
								field.Set(reflect.ValueOf(floatMap))
							case reflect.Bool:
								// map[string]bool
								boolMap := make(map[string]bool)
								for k, v := range mapData {
									if b, ok := v.(bool); ok {
										boolMap[k] = b
									}
								}
								field.Set(reflect.ValueOf(boolMap))
							case reflect.Slice:
								// map[string][]T
								if valueType.Elem().Kind() == reflect.String {
									// map[string][]string
									strSliceMap := make(map[string][]string)
									for k, v := range mapData {
										if slice, ok := v.([]interface{}); ok {
											strSlice := make([]string, len(slice))
											for i, item := range slice {
												if str, ok := item.(string); ok {
													strSlice[i] = str
												}
											}
											strSliceMap[k] = strSlice
										}
									}
									field.Set(reflect.ValueOf(strSliceMap))
								} else if valueType.Elem().Kind() == reflect.Int32 {
									// map[string][]int32
									intSliceMap := make(map[string][]int32)
									for k, v := range mapData {
										if slice, ok := v.([]interface{}); ok {
											intSlice := make([]int32, len(slice))
											for i, item := range slice {
												if num, ok := item.(float64); ok {
													intSlice[i] = int32(num)
												}
											}
											intSliceMap[k] = intSlice
										}
									}
									field.Set(reflect.ValueOf(intSliceMap))
								} else if valueType.Elem().Kind() == reflect.Float64 {
									// map[string][]float64
									floatSliceMap := make(map[string][]float64)
									for k, v := range mapData {
										if slice, ok := v.([]interface{}); ok {
											floatSlice := make([]float64, len(slice))
											for i, item := range slice {
												if num, ok := item.(float64); ok {
													floatSlice[i] = num
												}
											}
											floatSliceMap[k] = floatSlice
										}
									}
									field.Set(reflect.ValueOf(floatSliceMap))
								} else {
									// 其他类型的切片，使用interface{}
									field.Set(reflect.ValueOf(mapData))
								}
							default:
								// 其他复杂类型，直接设置为 interface{}
								field.Set(reflect.ValueOf(mapData))
							}
						} else {
							log.Error("{{.StructName}}转换错误: %v %v %v", convertTo{{.StructName}}, config, value)
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
func Get{{.StructName}}Config(id {{.IdType}}) (*{{.StructName}}, bool) {
	// 先从缓存获取
	if itemData, exists := {{.StructName}}CacheInstance.get{{.StructName}}FromCache(id); exists {
		return New{{.StructName}}(itemData), true
	}
	
	// 缓存未命中，从原始配置获取
	config, exists := config.GetConfig("{{.FileName}}", id)
	if !exists {
		return nil, false
	}

	// 转换为结构体
	if itemData, ok := convertTo{{.StructName}}(config); ok {
		// 设置到缓存
		{{.StructName}}CacheInstance.set{{.StructName}}ToCache(id, itemData)
		return New{{.StructName}}(itemData), true
	}

	return nil, false
}

// GetAll{{.StructName}}Configs 获取所有{{.FileName}}配置（带缓存）
func GetAll{{.StructName}}Configs() (map[{{.IdType}}]*{{.StructName}}, bool) {
	configs, exists := config.GetAllConfigs("{{.FileName}}")
	if !exists {
		return nil, false
	}

	result := make(map[{{.IdType}}]*{{.StructName}})
	for idInterface := range configs {
		// 类型转换 - 现在ID已经是正确的类型了
		var id {{.IdType}}
		var ok bool
		{{if eq .IdType "string"}}
		id, ok = idInterface.(string)
		{{else if eq .IdType "int32"}}
		id, ok = idInterface.(int32)
		{{else if eq .IdType "float64"}}
		id, ok = idInterface.(float64)
		{{end}}
		
		if !ok {
			continue // 类型转换失败，跳过
		}
		
		if item, ok := Get{{.StructName}}Config(id); ok {
			result[id] = item
		}
	}

	return result, true
}

// Reload{{.StructName}}Config 重新加载{{.FileName}}配置并清空缓存
func Reload{{.StructName}}Config() error {
	// 清空缓存
	{{.StructName}}CacheInstance.clear{{.StructName}}Cache()
	
	// 重新加载配置
	return config.ReloadConfig("{{.FileName}}")
}

// Validate{{.StructName}}Config 验证{{.FileName}}配置
func Validate{{.StructName}}Config(id {{.IdType}}) error {
	if _, exists := Get{{.StructName}}Config(id); !exists {
		return fmt.Errorf("配置不存在: %v", id)
	}
	return nil
}

// Clear{{.StructName}}Cache 手动清空{{.FileName}}配置缓存
func Clear{{.StructName}}Cache() {
	{{.StructName}}CacheInstance.clear{{.StructName}}Cache()
}
`

// GameConst专用模板 - 生成全局变量
const gameConstTemplate = `package config

import (
	"gameserver/common/config"
	"reflect"
	"strings"
	"sync"
	"time"
	"gameserver/core/log"
)

// GameConst 全局配置变量（自动生成，请勿手动修改）
var GameConst struct {
{{range .Fields}}	{{.Name}} {{.Type}} // {{.Comment}}
{{end}}}

var gameConstOnce sync.Once
var gameConstLoaded bool

func init() {
	initGameConst()
}

// initGameConst 初始化GameConst配置
func initGameConst() {
	go gameConstOnce.Do(func() {
		// 等待配置管理器初始化
		for {
			if !config.IsInitialized() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			configData, exists := config.GetConfig("GameConst.json", nil)
			if exists {
				// 配置管理器已初始化，继续处理
				formatData := make(map[string]interface{})
				for k, v := range configData.(map[string]interface{}) {
					formatData[strings.ToLower(k)] = v
				}
				processGameConstConfig(formatData)
				break
			}
			// 如果配置管理器未初始化，等待一下
			time.Sleep(10 * time.Millisecond)
		}
	})
}

// processGameConstConfig 处理GameConst配置数据
func processGameConstConfig(formatData map[string]interface{}) {
	// 使用反射设置字段值
	gameConstValue := reflect.ValueOf(&GameConst).Elem()
	gameConstType := gameConstValue.Type()

	for i := 0; i < gameConstValue.NumField(); i++ {
		field := gameConstValue.Field(i)
		fieldType := gameConstType.Field(i)
		fieldName := fieldType.Name

		// 将字段名转换为JSON字段名（首字母小写）
		jsonName := strings.ToLower(fieldName)

		if value, exists := formatData[jsonName]; exists {
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
					// 处理不同类型的切片
					elemType := field.Type().Elem()
					switch elemType.Kind() {
					case reflect.String:
						strSlice := make([]string, len(slice))
						for j, item := range slice {
							if str, ok := item.(string); ok {
								strSlice[j] = str
							}
						}
						field.Set(reflect.ValueOf(strSlice))
					case reflect.Int32:
						intSlice := make([]int32, len(slice))
						for j, item := range slice {
							if num, ok := item.(float64); ok {
								intSlice[j] = int32(num)
							}
						}
						field.Set(reflect.ValueOf(intSlice))
					case reflect.Float64:
						floatSlice := make([]float64, len(slice))
						for j, item := range slice {
							if num, ok := item.(float64); ok {
								floatSlice[j] = num
							}
						}
						field.Set(reflect.ValueOf(floatSlice))
					case reflect.Bool:
						boolSlice := make([]bool, len(slice))
						for j, item := range slice {
							if b, ok := item.(bool); ok {
								boolSlice[j] = b
							}
						}
						field.Set(reflect.ValueOf(boolSlice))
					case reflect.Slice:
						// 处理二维数组
						if elemType.Elem().Kind() == reflect.String {
							// [][]string
							str2DSlice := make([][]string, len(slice))
							for i, outerItem := range slice {
								if outerSlice, ok := outerItem.([]interface{}); ok {
									strSlice := make([]string, len(outerSlice))
									for j, innerItem := range outerSlice {
										if str, ok := innerItem.(string); ok {
											strSlice[j] = str
										}
									}
									str2DSlice[i] = strSlice
								}
							}
							field.Set(reflect.ValueOf(str2DSlice))
						} else if elemType.Elem().Kind() == reflect.Int32 {
							// [][]int32
							int2DSlice := make([][]int32, len(slice))
							for i, outerItem := range slice {
								if outerSlice, ok := outerItem.([]interface{}); ok {
									intSlice := make([]int32, len(outerSlice))
									for j, innerItem := range outerSlice {
										if num, ok := innerItem.(float64); ok {
											intSlice[j] = int32(num)
										}
									}
									int2DSlice[i] = intSlice
								}
							}
							field.Set(reflect.ValueOf(int2DSlice))
						} else if elemType.Elem().Kind() == reflect.Float64 {
							// [][]float64
							float2DSlice := make([][]float64, len(slice))
							for i, outerItem := range slice {
								if outerSlice, ok := outerItem.([]interface{}); ok {
									floatSlice := make([]float64, len(outerSlice))
									for j, innerItem := range outerSlice {
										if num, ok := innerItem.(float64); ok {
											floatSlice[j] = num
										}
									}
									float2DSlice[i] = floatSlice
								}
							}
							field.Set(reflect.ValueOf(float2DSlice))
						} else {
							// 其他二维数组类型，使用interface{}
							field.Set(reflect.ValueOf(slice))
						}
					default:
						// 对于复杂类型，直接设置为 interface{}
						field.Set(reflect.ValueOf(slice))
					}
				}
			case reflect.Map:
				if mapData, ok := value.(map[string]interface{}); ok {
					// 处理不同类型的map
					keyType := field.Type().Key()
					valueType := field.Type().Elem()

					// 检查key类型是否为string
					if keyType.Kind() == reflect.String {
						switch valueType.Kind() {
						case reflect.String:
							// map[string]string
							strMap := make(map[string]string)
							for k, v := range mapData {
								if str, ok := v.(string); ok {
									strMap[k] = str
								}
							}
							field.Set(reflect.ValueOf(strMap))
						case reflect.Int32:
							// map[string]int32
							intMap := make(map[string]int32)
							for k, v := range mapData {
								if num, ok := v.(float64); ok {
									intMap[k] = int32(num)
								}
							}
							field.Set(reflect.ValueOf(intMap))
						case reflect.Float64:
							// map[string]float64
							floatMap := make(map[string]float64)
							for k, v := range mapData {
								if num, ok := v.(float64); ok {
									floatMap[k] = num
								}
							}
							field.Set(reflect.ValueOf(floatMap))
						case reflect.Bool:
							// map[string]bool
							boolMap := make(map[string]bool)
							for k, v := range mapData {
								if b, ok := v.(bool); ok {
									boolMap[k] = b
								}
							}
							field.Set(reflect.ValueOf(boolMap))
						case reflect.Slice:
							// map[string][]T
							if valueType.Elem().Kind() == reflect.String {
								// map[string][]string
								strSliceMap := make(map[string][]string)
								for k, v := range mapData {
									if slice, ok := v.([]interface{}); ok {
										strSlice := make([]string, len(slice))
										for i, item := range slice {
											if str, ok := item.(string); ok {
												strSlice[i] = str
											}
										}
										strSliceMap[k] = strSlice
									}
								}
								field.Set(reflect.ValueOf(strSliceMap))
							} else if valueType.Elem().Kind() == reflect.Int32 {
								// map[string][]int32
								intSliceMap := make(map[string][]int32)
								for k, v := range mapData {
									if slice, ok := v.([]interface{}); ok {
										intSlice := make([]int32, len(slice))
										for i, item := range slice {
											if num, ok := item.(float64); ok {
												intSlice[i] = int32(num)
											}
										}
										intSliceMap[k] = intSlice
									}
								}
								field.Set(reflect.ValueOf(intSliceMap))
							} else if valueType.Elem().Kind() == reflect.Float64 {
								// map[string][]float64
								floatSliceMap := make(map[string][]float64)
								for k, v := range mapData {
									if slice, ok := v.([]interface{}); ok {
										floatSlice := make([]float64, len(slice))
										for i, item := range slice {
											if num, ok := item.(float64); ok {
												floatSlice[i] = num
											}
										}
										floatSliceMap[k] = floatSlice
									}
								}
								field.Set(reflect.ValueOf(floatSliceMap))
							} else {
								// 其他类型的切片，使用interface{}
								field.Set(reflect.ValueOf(mapData))
							}
						default:
							// 其他复杂类型，直接设置为 interface{}
							field.Set(reflect.ValueOf(mapData))
						}
					} else {
						log.Error("GameConst转换错误: %v %v %v", fieldName, value)
					}
				}
			}
		}
	}
	gameConstLoaded = true
}


// ReloadGameConst 重新加载GameConst配置
func ReloadGameconstConfig() error {
	// 重新加载配置
	if err := config.ReloadConfig("GameConst.json"); err != nil {
		return err
	}
	
	// 重置once，允许重新初始化
	gameConstOnce = sync.Once{}
	gameConstLoaded = false
	
	// 重新初始化
	initGameConst()
	
	return nil
}

// IsGameConstLoaded 检查GameConst是否已加载
func IsGameConstLoaded() bool {
	if !gameConstLoaded {
		initGameConst()
	}
	return gameConstLoaded
}

`

func main() {
	// 从配置文件读取路径配置
	configDir, outputDir, err := loadConfig()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 默认配置目录
	defaultConfigDir := "../../conf/config"

	// 如果配置目录不是默认目录，需要复制文件到默认目录
	if configDir != defaultConfigDir {
		fmt.Printf("配置目录不是默认目录，正在复制JSON文件...\n")
		fmt.Printf("源目录: %s\n", configDir)
		fmt.Printf("目标目录: %s\n", defaultConfigDir)

		// 创建默认配置目录
		if err := os.MkdirAll(defaultConfigDir, 0755); err != nil {
			log.Fatalf("创建默认配置目录失败: %v", err)
		}

		// 复制JSON文件
		if err := copyJSONFiles(configDir, defaultConfigDir); err != nil {
			log.Fatalf("复制JSON文件失败: %v", err)
		}

		// 使用默认配置目录
		configDir = defaultConfigDir
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

		// 生成Wrapper代码（如果不存在）
		// GameConst.json 不需要生成wrapper
		if configInfo.FileName != "GameConst.json" {
			if err := generateWrapperFile(configInfo, outputDir); err != nil {
				log.Printf("生成Wrapper文件失败 %s: %v", fileName, err)
				// Wrapper生成失败不影响主流程，继续执行
			}
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
	if data, err := os.ReadFile(configFile); err == nil {
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

// copyJSONFiles 复制JSON文件从源目录到目标目录
func copyJSONFiles(srcDir, dstDir string) error {
	// 扫描源目录中的所有JSON文件
	files, err := filepath.Glob(filepath.Join(srcDir, "*.json"))
	if err != nil {
		return fmt.Errorf("扫描源目录失败: %v", err)
	}

	if len(files) == 0 {
		fmt.Printf("源目录中没有找到JSON文件: %s\n", srcDir)
		return nil
	}

	fmt.Printf("找到 %d 个JSON文件需要复制\n", len(files))

	// 复制每个JSON文件
	for _, srcFile := range files {
		fileName := filepath.Base(srcFile)
		dstFile := filepath.Join(dstDir, fileName)

		// 读取源文件
		srcData, err := os.ReadFile(srcFile)
		if err != nil {
			return fmt.Errorf("读取源文件失败 %s: %v", srcFile, err)
		}

		// 写入目标文件
		if err := os.WriteFile(dstFile, srcData, 0644); err != nil {
			return fmt.Errorf("写入目标文件失败 %s: %v", dstFile, err)
		}

		fmt.Printf("已复制: %s -> %s\n", fileName, dstFile)
	}

	return nil
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
	fields := extractFields(jsonData, fileName == "GameConst.json")

	// 检测ID字段类型
	idType := detectIdType(jsonData)

	// 生成结构体名称
	structName := generateStructName(fileName)

	return &ConfigInfo{
		FileName:    fileName,
		StructName:  structName,
		PackageName: "config",
		Fields:      fields,
		SampleData:  jsonData,
		IdType:      idType,
	}, nil
}

// extractFields 从JSON数据中提取字段信息
func extractFields(data interface{}, isGameConst bool) []FieldInfo {
	var allFields []FieldInfo
	fieldMap := make(map[string]FieldInfo) // 用于去重和合并字段信息

	switch v := data.(type) {
	case []interface{}:
		// 遍历所有对象，收集所有字段
		for _, item := range v {
			if mapData, ok := item.(map[string]interface{}); ok {
				fields := extractFieldsFromMap(mapData, isGameConst)
				// 合并字段信息，如果字段已存在，保留更具体的类型信息
				for _, field := range fields {
					if existingField, exists := fieldMap[field.JSONName]; exists {
						// 如果新字段类型更具体（不是interface{}），则更新
						if field.Type != "interface{}" && existingField.Type == "interface{}" {
							fieldMap[field.JSONName] = field
						}
					} else {
						fieldMap[field.JSONName] = field
					}
				}
			}
		}
	case map[string]interface{}:
		// 检查是否是新的配置格式：最外层key是配置ID，value是配置对象
		if isConfigMapFormat(v) {
			// 遍历所有配置项，收集所有字段
			for _, configItem := range v {
				if mapData, ok := configItem.(map[string]interface{}); ok {
					fields := extractFieldsFromMap(mapData, isGameConst)
					// 合并字段信息，如果字段已存在，保留更具体的类型信息
					for _, field := range fields {
						if existingField, exists := fieldMap[field.JSONName]; exists {
							// 如果新字段类型更具体（不是interface{}），则更新
							if field.Type != "interface{}" && existingField.Type == "interface{}" {
								fieldMap[field.JSONName] = field
							}
						} else {
							fieldMap[field.JSONName] = field
						}
					}
				}
			}
		} else {
			// 原有的格式：直接是配置对象
			fields := extractFieldsFromMap(v, isGameConst)
			for _, field := range fields {
				fieldMap[field.JSONName] = field
			}
		}
	}

	// 将map转换为slice
	for _, field := range fieldMap {
		allFields = append(allFields, field)
	}

	// 按字段顺序排序
	sort.Slice(allFields, func(i, j int) bool {
		return allFields[i].Order < allFields[j].Order
	})

	return allFields
}

// extractFieldsFromMap 从map中提取字段信息
func extractFieldsFromMap(data map[string]interface{}, isGameConst bool) []FieldInfo {
	var fields []FieldInfo
	order := 0

	// 定义字段优先级顺序
	priorityFields := []string{"id", "name", "type", "level", "hp", "attack", "defense", "exp", "price", "durability", "description"}

	// 按优先级处理字段
	for _, priorityKey := range priorityFields {
		if value, exists := data[priorityKey]; exists {
			var fieldName string
			if isGameConst {
				fieldName = toGameConstFieldName(priorityKey)
			} else {
				fieldName = toCamelCase(priorityKey)
			}

			fields = append(fields, FieldInfo{
				Name:     fieldName,
				JSONName: priorityKey,
				Type:     inferGoType(value, priorityKey),
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
			var fieldName string
			if isGameConst {
				fieldName = toGameConstFieldName(key)
			} else {
				fieldName = toCamelCase(key)
			}

			field := FieldInfo{
				Name:     fieldName,
				JSONName: key,
				Type:     inferGoType(value, key),
				Comment:  key,
				Order:    order,
			}
			fields = append(fields, field)
			order++
		}
	}

	return fields
}

// isConfigMapFormat 检查是否是新的配置格式（最外层key是配置ID）
func isConfigMapFormat(data map[string]interface{}) bool {
	// 如果map为空，不是配置格式
	if len(data) == 0 {
		return false
	}

	// 检查第一个值是否是对象
	for _, value := range data {
		if _, ok := value.(map[string]interface{}); ok {
			// 如果包含id字段，或者是没有id字段但使用map key作为ID的格式，都认为是配置格式
			// 这样无论有没有id字段，都使用map的key作为配置ID
			return true
		}
		break // 只检查第一个值
	}

	return false
}

// detectIdType 检测ID字段的类型
func detectIdType(data interface{}) string {
	switch v := data.(type) {
	case []interface{}:
		if len(v) > 0 {
			if mapData, ok := v[0].(map[string]interface{}); ok {
				if idValue, exists := mapData["id"]; exists {
					return inferGoType(idValue, "id")
				}
			}
		}
	case map[string]interface{}:
		// 检查是否是新的配置格式
		if isConfigMapFormat(v) {
			// 对于新格式，ID类型是string（因为最外层的key是字符串）
			if v, ok := data.(map[string]interface{}); ok {
				for key := range v {
					if _, err := strconv.Atoi(key); err != nil {
						return "string"
					}
				}
			}
			return "int32"
		} else {
			// 原有格式：直接检查id字段
			if idValue, exists := v["id"]; exists {
				return inferGoType(idValue, "id")
			}
		}
	}
	// 默认返回string类型
	return "string"
}

// inferGoType 推断Go类型
func inferGoType(value interface{}, fieldName string) string {
	switch v := value.(type) {
	case string:
		return "string"
	case float64:
		// 对于所有数值字段，如果值是整数（没有小数点），使用int32
		if v == float64(int64(v)) {
			return "int32"
		}
		// 只有真正有小数点的数值才使用float64
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
				// 检查是否为整数数组
				allInts := true
				for _, item := range v {
					if num, ok := item.(float64); !ok || num != float64(int64(num)) {
						allInts = false
						break
					}
				}
				if allInts {
					return "[]int32"
				}
				return "[]float64"
			case []interface{}:
				// 二维数组，检查内层数组的元素类型
				if len(v[0].([]interface{})) > 0 {
					switch v[0].([]interface{})[0].(type) {
					case string:
						return "[][]string"
					case float64:
						// 检查是否为整数二维数组
						allInts := true
						for _, outerItem := range v {
							if outerSlice, ok := outerItem.([]interface{}); ok {
								for _, innerItem := range outerSlice {
									if num, ok := innerItem.(float64); !ok || num != float64(int64(num)) {
										allInts = false
										break
									}
								}
							}
							if !allInts {
								break
							}
						}
						if allInts {
							return "[][]int32"
						}
						return "[][]float64"
					default:
						return "[][]interface{}"
					}
				}
				return "[][]interface{}"
			case map[string]interface{}:
				// 对于复杂对象数组，使用interface{}
				return "[]interface{}"
			default:
				return "[]interface{}"
			}
		}
		return "[]interface{}"
	case map[string]interface{}:
		// 分析map的值类型，尝试推断更具体的类型
		if len(v) == 0 {
			return "map[string]int32"
		}

		// 检查所有值的类型是否一致
		var valueType string
		allSameType := true

		for _, mapValue := range v {
			currentType := inferGoType(mapValue, "")
			if valueType == "" {
				valueType = currentType
			} else if valueType != currentType {
				allSameType = false
				break
			}
		}

		// 如果所有值都是相同类型，生成具体的map类型
		if allSameType && valueType != "interface{}" {
			return "map[string]" + valueType
		}

		// 否则使用通用类型
		return "map[string]interface{}"
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
		if len(part) > 0 {
			parts[i] = titleCase(part)
		}
	}
	return strings.Join(parts, "")
}

// titleCase 将字符串首字母大写（替代已弃用的 strings.Title）
func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	caser := cases.Title(language.English)
	return caser.String(s)
}

// toGameConstFieldName 为GameConst生成字段名
func toGameConstFieldName(s string) string {
	// 特殊处理GameConst的字段名
	switch s {
	case "shareReward":
		return "ShareReward"
	case "sidebarReward":
		return "SidebarReward"
	case "sidebarRewardName":
		return "SidebarRewardName"
	case "gameIcon":
		return "GameIcon"
	case "equipComposeNum":
		return "EquipComposeNum"
	default:
		// 对于其他字段，使用普通的驼峰命名
		return toCamelCase(s)
	}
}

// generateStructName 生成结构体名称
func generateStructName(fileName string) string {
	// 移除.json扩展名
	name := strings.TrimSuffix(fileName, ".json")

	// 转换为单数形式并首字母大写
	name = strings.TrimSuffix(name, "s")

	return titleCase(name)
}

// generateGoFile 生成Go文件
func generateGoFile(configInfo *ConfigInfo, outputDir string) error {
	var tmpl *template.Template
	var err error

	// 根据文件名选择不同的模板
	if configInfo.FileName == "GameConst.json" {
		// 使用GameConst专用模板
		tmpl, err = template.New("gameconst").Parse(gameConstTemplate)
	} else {
		// 使用普通模板
		tmpl, err = template.New("config").Parse(goTemplate)
	}

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

// Wrapper模板 - 简化版本，提供基础框架
const wrapperTemplate = `package wrapper

import (
	"fmt"
	config "gameserver/common/config/generated"
	"gameserver/core/log"
	"sync"
	"time"
)

// {{.StructName}}缓存管理
var (
	load{{.StructName}}Mutex  sync.RWMutex
	{{.StructName}}Loaded     bool
	{{.StructName}}Loading    bool  // 防止重复加载
	{{.StructName}}LoadError  error // 记录加载错误
)

// Load{{.StructName}}Cache 加载所有{{.StructName}}到缓存（带重试机制）
func Load{{.StructName}}Cache() error {
	load{{.StructName}}Mutex.Lock()
	defer load{{.StructName}}Mutex.Unlock()

	// 如果已经加载过，直接返回
	if {{.StructName}}Loaded {
		return nil
	}

	// 如果正在加载中，等待加载完成
	if {{.StructName}}Loading {
		// 释放锁，等待其他线程完成加载
		load{{.StructName}}Mutex.Unlock()
		time.Sleep(10 * time.Millisecond)
		load{{.StructName}}Mutex.Lock()

		// 再次检查是否已加载
		if {{.StructName}}Loaded {
			return nil
		}
		if {{.StructName}}LoadError != nil {
			return {{.StructName}}LoadError
		}
	}

	// 标记为正在加载
	{{.StructName}}Loading = true
	{{.StructName}}LoadError = nil

	// 重试机制：最多重试3次
	maxRetries := 3
	for attempt := 0; attempt < maxRetries; attempt++ {
		// 获取所有{{.StructName}}配置
		all{{.StructName}}, exists := config.GetAll{{.StructName}}Configs()
		if !exists {
			if attempt < maxRetries-1 {
				// 等待一段时间后重试
				time.Sleep(time.Duration(attempt+1) * 50 * time.Millisecond)
				continue
			}
			{{.StructName}}Loading = false
			{{.StructName}}LoadError = fmt.Errorf("无法获取{{.StructName}}配置，已重试%d次", maxRetries)
			return {{.StructName}}LoadError
		}

		log.Debug("加载{{.StructName}}配置: %d", len(all{{.StructName}}))
		// TODO: 在这里添加自定义的缓存处理逻辑
		// 例如：
		// - 初始化特定的缓存结构
		// - 根据业务需求筛选和预处理数据
		// - 设置特殊的缓存策略
		// 
		// 示例代码：
		// for id, item := range all{{.StructName}} {
		//     // 自定义处理逻辑
		//     // 例如：根据特定条件筛选数据
		//     // 或者：预处理数据格式
		// }

		// 加载成功
		{{.StructName}}Loaded = true
		{{.StructName}}Loading = false
		{{.StructName}}LoadError = nil
		return nil
	}

	// 如果所有重试都失败了
	{{.StructName}}Loading = false
	{{.StructName}}LoadError = fmt.Errorf("加载{{.StructName}}失败，已重试%d次", maxRetries)
	return {{.StructName}}LoadError
}

// Clear{{.StructName}}Cache 清空{{.StructName}}缓存
func Clear{{.StructName}}Cache() {
	load{{.StructName}}Mutex.Lock()
	defer load{{.StructName}}Mutex.Unlock()

	// TODO: 在这里添加自定义的缓存清理逻辑
	// 例如：
	// - 清理特定的缓存结构
	// - 重置相关的状态变量
	// - 释放相关资源

	{{.StructName}}Loaded = false
	{{.StructName}}Loading = false
	{{.StructName}}LoadError = nil
}

// Reload{{.StructName}}Cache 重新加载{{.StructName}}缓存
func Reload{{.StructName}}Cache() error {
	Clear{{.StructName}}Cache()
	return Load{{.StructName}}Cache()
}

// Ensure{{.StructName}}CacheLoaded 确保{{.StructName}}缓存已加载（手动触发加载）
func Ensure{{.StructName}}CacheLoaded() error {
	return Load{{.StructName}}Cache()
}

// Get{{.StructName}}LoadStatus 获取{{.StructName}}加载状态
func Get{{.StructName}}LoadStatus() (loaded bool, loading bool, err error) {
	load{{.StructName}}Mutex.RLock()
	defer load{{.StructName}}Mutex.RUnlock()

	return {{.StructName}}Loaded, {{.StructName}}Loading, {{.StructName}}LoadError
}
`

// generateWrapperFile 生成Wrapper文件
func generateWrapperFile(configInfo *ConfigInfo, outputDir string) error {
	// 检查是否已经存在wrapper文件
	wrapperDir := filepath.Join(outputDir, "wrapper")
	wrapperFileName := strings.ToLower(configInfo.StructName) + "_wrapper.go"
	wrapperFilePath := filepath.Join(wrapperDir, wrapperFileName)

	// 如果wrapper文件已存在，跳过生成
	if _, err := os.Stat(wrapperFilePath); err == nil {
		fmt.Printf("Wrapper文件已存在，跳过生成: %s\n", wrapperFileName)
		return nil
	}

	// 创建wrapper目录（如果不存在）
	if err := os.MkdirAll(wrapperDir, 0755); err != nil {
		return fmt.Errorf("创建wrapper目录失败: %v", err)
	}

	// 创建模板
	tmpl, err := template.New("wrapper").Parse(wrapperTemplate)
	if err != nil {
		return err
	}

	// 创建输出文件
	file, err := os.Create(wrapperFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 执行模板
	if err := tmpl.Execute(file, configInfo); err != nil {
		return err
	}

	fmt.Printf("成功生成Wrapper: %s\n", wrapperFileName)
	return nil
}
