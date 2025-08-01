package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// MethodInfo 存储方法信息
type MethodInfo struct {
	Name     string
	Receiver string
	Params   []ParamInfo
	Returns  []string
}

// ParamInfo 存储参数信息
type ParamInfo struct {
	Name string
	Type string
}

// MethodGenerator 通用方法生成器
type MethodGenerator struct {
	SourceFile  string
	OutputFile  string
	StructName  string
	PackageName string
}

// NewMethodGenerator 创建新的生成器
func NewMethodGenerator(sourceFile, outputFile, structName, packageName string) *MethodGenerator {
	return &MethodGenerator{
		SourceFile:  sourceFile,
		OutputFile:  outputFile,
		StructName:  structName,
		PackageName: packageName,
	}
}

// AutoDetectStructs 自动检测目录下的所有结构体
func AutoDetectStructs(sourceDir string) ([]string, string, error) {
	var structs []string
	var packageName string
	
	// 遍历目录下的所有.go文件
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			// 解析文件
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			
			// 获取包名
			if packageName == "" {
				packageName = node.Name.Name
			}
			
			// 提取结构体
			ast.Inspect(node, func(n ast.Node) bool {
				if typeDecl, ok := n.(*ast.TypeSpec); ok {
					if structType, ok := typeDecl.Type.(*ast.StructType); ok {
						// 检查是否嵌入了ActorMessageHandler
						for _, field := range structType.Fields.List {
							if selectorExpr, ok := field.Type.(*ast.SelectorExpr); ok {
								if ident, ok := selectorExpr.X.(*ast.Ident); ok {
									if ident.Name == "actor_manager" && selectorExpr.Sel.Name == "ActorMessageHandler" {
										structs = append(structs, typeDecl.Name.Name)
										break
									}
								}
							}
						}
					}
				}
				return true
			})
		}
		return nil
	})
	
	return structs, packageName, err
}

// Generate 生成代码
func (g *MethodGenerator) Generate() error {
	// 解析源目录下的所有文件
	fset := token.NewFileSet()
	var allMethods []MethodInfo
	
	// 遍历目录下的所有.go文件
	err := filepath.Walk(g.SourceFile, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			
			// 提取指定结构体的方法
			methods := g.extractMethods(node)
			allMethods = append(allMethods, methods...)
		}
		return nil
	})
	
	if err != nil {
		return fmt.Errorf("解析文件失败: %v", err)
	}

	if len(allMethods) == 0 {
		return fmt.Errorf("未找到%s结构体的方法", g.StructName)
	}

	// 生成代码
	return g.generateCode(allMethods)
}

// extractMethods 提取结构体的方法
func (g *MethodGenerator) extractMethods(node *ast.File) []MethodInfo {
	var methods []MethodInfo

	ast.Inspect(node, func(n ast.Node) bool {
		if funcDecl, ok := n.(*ast.FuncDecl); ok {
			if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
				// 检查接收者是否为指定结构体
				recvType := funcDecl.Recv.List[0].Type
				if starExpr, ok := recvType.(*ast.StarExpr); ok {
					if ident, ok := starExpr.X.(*ast.Ident); ok && ident.Name == g.StructName {
						method := MethodInfo{
							Name:     funcDecl.Name.Name,
							Receiver: g.StructName,
						}

						// 提取参数
						if funcDecl.Type.Params != nil {
							for _, param := range funcDecl.Type.Params.List {
								paramType := fmt.Sprintf("%v", param.Type)
								for _, name := range param.Names {
									method.Params = append(method.Params, ParamInfo{
										Name: name.Name,
										Type: paramType,
									})
								}
							}
						}

						// 提取返回值
						if funcDecl.Type.Results != nil {
							for _, result := range funcDecl.Type.Results.List {
								if ident, ok := result.Type.(*ast.Ident); ok {
									method.Returns = append(method.Returns, ident.Name)
								}
							}
						}

						methods = append(methods, method)
					}
				}
			}
		}
		return true
	})

	return methods
}

// generateCode 生成代码
func (g *MethodGenerator) generateCode(methods []MethodInfo) error {
	// 创建输出目录
	outputDir := filepath.Dir(g.OutputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	// 创建输出文件
	file, err := os.Create(g.OutputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer file.Close()

	// 生成代码模板
	tmpl := template.Must(template.New("methods").Parse(`
package {{.PackageName}}

import (
	actor_manager "gameserver/core/actor"
)

{{range .Methods}}
// {{.Name}} 调用{{$.StructName}}的{{.Name}}方法
func {{.Name}}({{$.StructName}}Id int64{{if .Params}}, {{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}} {{$param.Type}}{{end}}{{end}}) {
	args := []interface{}{}
	{{if .Params}}{{range .Params}}args = append(args, {{.Name}})
	{{end}}{{end}}
	actor_manager.Send[{{$.StructName}}]({{$.StructName}}Id, "{{.Name}}", args)
}
{{end}}
`))

	// 执行模板
	data := struct {
		StructName  string
		PackageName string
		Methods     []MethodInfo
	}{
		StructName:  g.StructName,
		PackageName: g.PackageName,
		Methods:     methods,
	}
	return tmpl.Execute(file, data)
}

// GenerateFromFile 从指定文件生成代码
func GenerateFromFile(sourceFile, outputFile, structName, packageName string) error {
	generator := NewMethodGenerator(sourceFile, outputFile, structName, packageName)
	return generator.Generate()
}
