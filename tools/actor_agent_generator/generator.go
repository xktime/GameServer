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

type MethodInfo struct {
	Name           string
	Receiver       string
	Params         []ParamInfo
	Returns        []string
	SingleReturn   bool
	Returns0       string
	DefaultReturns []string
	IsGenericArgs  bool
}

type ParamInfo struct {
	Name string
	Type string
}

type StructInfo struct {
	Name     string
	Package  string
	FilePath string
}

type MethodGenerator struct {
	SourceFile  string
	OutputFile  string
	StructName  string
	PackageName string
}

func NewMethodGenerator(sourceFile, outputFile, structName, packageName string) *MethodGenerator {
	return &MethodGenerator{
		SourceFile:  sourceFile,
		OutputFile:  outputFile,
		StructName:  structName,
		PackageName: packageName,
	}
}

func AutoDetectStructs(sourceDir string) ([]StructInfo, error) {
	var structs []StructInfo

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_actor.go") {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}

			ast.Inspect(node, func(n ast.Node) bool {
				if typeDecl, ok := n.(*ast.TypeSpec); ok {
					if structType, ok := typeDecl.Type.(*ast.StructType); ok {
						for _, field := range structType.Fields.List {
							if selectorExpr, ok := field.Type.(*ast.SelectorExpr); ok {
								if ident, ok := selectorExpr.X.(*ast.Ident); ok {
									if ident.Name == "actor_manager" && selectorExpr.Sel.Name == "ActorMessageHandler" {
										structs = append(structs, StructInfo{
											Name:     typeDecl.Name.Name,
											Package:  node.Name.Name,
											FilePath: path,
										})
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

	return structs, err
}

func (g *MethodGenerator) Generate() error {
	fset := token.NewFileSet()
	var allMethods []MethodInfo

	err := filepath.Walk(g.SourceFile, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_actor.go") {
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}

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

	return g.generateCode(allMethods)
}

func (g *MethodGenerator) extractMethods(node *ast.File) []MethodInfo {
	var methods []MethodInfo

	ast.Inspect(node, func(n ast.Node) bool {
		if funcDecl, ok := n.(*ast.FuncDecl); ok {
			if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
				recvType := funcDecl.Recv.List[0].Type
				if starExpr, ok := recvType.(*ast.StarExpr); ok {
					if ident, ok := starExpr.X.(*ast.Ident); ok && ident.Name == g.StructName {
						method := MethodInfo{
							Name:     funcDecl.Name.Name,
							Receiver: g.StructName,
						}

						if funcDecl.Type.Params != nil {
							for _, param := range funcDecl.Type.Params.List {
								paramType := g.formatType(param.Type)
								for _, name := range param.Names {
									method.Params = append(method.Params, ParamInfo{
										Name: name.Name,
										Type: paramType,
									})
								}
							}
						}

						if funcDecl.Type.Results != nil {
							for _, result := range funcDecl.Type.Results.List {
								returnType := g.formatType(result.Type)
								method.Returns = append(method.Returns, returnType)
							}
						}

						// 设置SingleReturn和Returns0字段
						if len(method.Returns) == 1 {
							method.SingleReturn = true
							method.Returns0 = method.Returns[0]
						}

						// 设置默认返回值
						for _, ret := range method.Returns {
							if ret == "bool" {
								method.DefaultReturns = append(method.DefaultReturns, "false")
							} else {
								method.DefaultReturns = append(method.DefaultReturns, ret+"{}")
							}
						}

						// 检查是否是通用的args []interface{}方法
						if len(method.Params) == 1 && method.Params[0].Type == "[]interface{}" && method.Params[0].Name == "args" {
							method.IsGenericArgs = true
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

func (g *MethodGenerator) formatType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + g.formatType(t.X)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name + "." + t.Sel.Name
		}
		return fmt.Sprintf("%v", expr)
	case *ast.ArrayType:
		return "[]" + g.formatType(t.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return fmt.Sprintf("%v", expr)
	}
}

func (g *MethodGenerator) generateCode(methods []MethodInfo) error {
	outputDir := filepath.Dir(g.OutputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %v", err)
	}

	file, err := os.Create(g.OutputFile)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %v", err)
	}
	defer file.Close()

	tmpl := template.Must(template.New("methods").Parse(`
package {{.PackageName}}

import (
	actor_manager "gameserver/core/actor"
	{{if .HasGate}}"gameserver/core/gate"{{end}}
	{{if .HasModels}}"gameserver/common/models"{{end}}
	{{if .HasMessage}}"gameserver/common/msg/message"{{end}}
	{{if .HasProto}}"google.golang.org/protobuf/proto"{{end}}
)

{{range .Methods}}
// {{.Name}} 调用{{$.StructName}}的{{.Name}}方法
func {{.Name}}({{$.StructName}}Id int64{{if .Params}}, {{range $index, $param := .Params}}{{if $index}}, {{end}}{{$param.Name}} {{$param.Type}}{{end}}{{end}}){{if .Returns}} ({{range $index, $return := .Returns}}{{if $index}}, {{end}}{{$return}}{{end}}){{end}} {
	{{if .IsGenericArgs}}{{if .Returns}}future := actor_manager.RequestFuture[{{$.StructName}}]({{$.StructName}}Id, "{{.Name}}", args)
	result, _ := future.Result()
	{{if .SingleReturn}}return result.({{.Returns0}}){{else}}if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) == {{len .Returns}} {
		{{range $index, $return := .Returns}}ret{{$index}} := resultSlice[{{$index}}].({{$return}})
		{{end}}return {{range $index, $return := .Returns}}{{if $index}}, {{end}}ret{{$index}}{{end}}
	}
	return {{range $index, $default := .DefaultReturns}}{{if $index}}, {{end}}{{$default}}{{end}}{{end}}{{else}}actor_manager.Send[{{$.StructName}}]({{$.StructName}}Id, "{{.Name}}", args){{end}}{{else}}{{if .Params}}sendArgs := []interface{}{}
	{{range .Params}}sendArgs = append(sendArgs, {{.Name}})
	{{end}}{{else}}sendArgs := []interface{}{}{{end}}
	{{if .Returns}}future := actor_manager.RequestFuture[{{$.StructName}}]({{$.StructName}}Id, "{{.Name}}", sendArgs)
	result, _ := future.Result()
	{{if .SingleReturn}}return result.({{.Returns0}}){{else}}if resultSlice, ok := result.([]interface{}); ok && len(resultSlice) == {{len .Returns}} {
		{{range $index, $return := .Returns}}ret{{$index}} := resultSlice[{{$index}}].({{$return}})
		{{end}}return {{range $index, $return := .Returns}}{{if $index}}, {{end}}ret{{$index}}{{end}}
	}
	return {{range $index, $default := .DefaultReturns}}{{if $index}}, {{end}}{{$default}}{{end}}{{end}}{{else}}actor_manager.Send[{{$.StructName}}]({{$.StructName}}Id, "{{.Name}}", sendArgs){{end}}{{end}}
}
{{end}}
`))

	hasGate := false
	hasModels := false
	hasFuture := false
	hasProto := false
	hasMessage := false
	for _, method := range methods {
		for _, param := range method.Params {
			if strings.Contains(param.Type, "gate.Agent") {
				hasGate = true
			}
			if strings.Contains(param.Type, "proto.Message") {
				hasProto = true
			}
			if strings.Contains(param.Type, "message.") {
				hasMessage = true
			}
		}
		for _, ret := range method.Returns {
			if strings.Contains(ret, "models.User") {
				hasModels = true
			}
		}
		if len(method.Returns) > 0 {
			hasFuture = true
		}
	}

	data := struct {
		StructName  string
		PackageName string
		Methods     []MethodInfo
		HasGate     bool
		HasModels   bool
		HasFuture   bool
		HasProto    bool
		HasMessage  bool
	}{
		StructName:  g.StructName,
		PackageName: g.PackageName,
		Methods:     methods,
		HasGate:     hasGate,
		HasModels:   hasModels,
		HasFuture:   hasFuture,
		HasProto:    hasProto,
		HasMessage:  hasMessage,
	}
	return tmpl.Execute(file, data)
}

func CheckStructHasMethods(sourceDir, structName string) (bool, error) {
	hasMethods := false
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_actor.go") {
			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}

			ast.Inspect(node, func(n ast.Node) bool {
				if funcDecl, ok := n.(*ast.FuncDecl); ok {
					if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
						recvType := funcDecl.Recv.List[0].Type
						if starExpr, ok := recvType.(*ast.StarExpr); ok {
							if ident, ok := starExpr.X.(*ast.Ident); ok && ident.Name == structName {
								hasMethods = true
								return false
							}
						}
					}
				}
				return true
			})
		}
		return nil
	})

	return hasMethods, err
}

func GenerateFromFile(sourceFile, outputFile, structName, packageName string) error {
	generator := NewMethodGenerator(sourceFile, outputFile, structName, packageName)
	return generator.Generate()
}
