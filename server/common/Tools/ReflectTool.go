package Tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
)

func GetStructListByDir(dirPath string) []string {
	files, _ := os.ReadDir(dirPath)
	var result []string
	for _, file := range files {
		structList, _ := GetStructListByFile(dirPath + file.Name())
		result = append(result, structList...)
	}
	return result
}

func GetStructListByFile(filePath string) ([]string, map[string][]string) {
	src, _ := os.ReadFile(filePath)
	fSet := token.NewFileSet()
	f, err := parser.ParseFile(fSet, "", src, 0)
	if err != nil {
		log.Fatal(err)
	}

	var structList []string
	methodMap := make(map[string][]string, 8) // struct -> list of methods
	for _, decl := range f.Decls {
		switch concreteDecl := decl.(type) {
		case *ast.FuncDecl:
			if concreteDecl.Recv == nil {
				// not method
				continue
			}

			var structName string
			for _, field := range concreteDecl.Recv.List {
				if starExpr, ok := field.Type.(*ast.StarExpr); ok {
					// receiver is pointer type
					structName = starExpr.X.(*ast.Ident).Name
				}
			}
			if len(structName) != 0 {
				methodMap[structName] = append(methodMap[structName], concreteDecl.Name.Name)
			}

		case *ast.GenDecl:
			for _, spec := range concreteDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				_, ok = typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				structList = append(structList, typeSpec.Name.Name)
			}
		}
	}

	return structList, methodMap
}
