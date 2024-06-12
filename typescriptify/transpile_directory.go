package typescriptify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

type arrayImports []string

func (i *arrayImports) String() string {
	return "// custom imports:\n\n" + strings.Join(*i, "\n")
}

func (i *arrayImports) Set(value string) error {
	*i = append(*i, value)
	return nil
}

const TEMPLATE = `package main

import (
	"fmt"

	m "{{ .ModelsPackage }}"
	"github.com/arpitbbhayani/transpiler/typescriptify"
)

func main() {
	t := typescriptify.New()
	t.CreateInterface = {{ .Interface }}
{{ range $key, $value := .InitParams }}	t.{{ $key }}={{ $value }}
{{ end }}
{{ range .Structs }}	t.Add({{ . }}{})
{{ end }}
{{ range .CustomImports }}	t.AddImport("{{ . }}")
{{ end }}
	err := t.ConvertToFile("{{ .TargetFile }}")
	if err != nil {
		panic(err.Error())
	}
	fmt.Println("OK")
}`

type Params struct {
	ModelsPackage string
	TargetFile    string
	Structs       []string
	InitParams    map[string]interface{}
	CustomImports arrayImports
	Interface     bool
	Verbose       bool
}

var structs []string

func init() {
	structs = []string{}
}

func visit(path string, fi fs.FileInfo, err error) error {
	if err != nil {
		fmt.Println(err)
		return err
	}
	if !strings.HasSuffix(path, ".go") {
		return nil
	}
	fileStructs, err := GetGolangFileStructs(path)
	if err != nil {
		panic(fmt.Sprintf("Error loading/parsing golang file %s: %s", path, err.Error()))
	}
	structs = append(structs, fileStructs...)
	return nil
}

func popuplateStructs(dirpath string) {
	err := filepath.Walk(dirpath, visit)
	if err != nil {
		panic(err)
	}
}

func TranspileDirectory(workingDir, packageDir, packagePath, outputFilepath string) {
	var p Params

	p.ModelsPackage = packagePath
	p.TargetFile = outputFilepath
	p.Interface = true

	if len(p.ModelsPackage) == 0 {
		fmt.Fprintln(os.Stderr, "No package given")
		os.Exit(1)
	}

	popuplateStructs(packageDir)
	fmt.Printf("Found %d structs in %s.\n", len(structs), p.TargetFile)

	t := template.Must(template.New("").Parse(TEMPLATE))

	temDirpath := filepath.Join(workingDir, fmt.Sprintf("transpiler_%d", time.Now().Unix()))
	fmt.Println(temDirpath)
	err := os.MkdirAll(temDirpath, 0766)
	if err != nil {
		panic(err)
	}
	defer func() {
		os.RemoveAll(temDirpath)
	}()

	filepath := filepath.Join(temDirpath, "transpiler.go")
	fp, err := os.Create(filepath)
	if err != nil {
		panic(err)
	}
	defer func() {
		os.Remove(filepath)
	}()
	defer fp.Close()

	structsArr := make([]string, 0)
	for _, str := range structs {
		str = strings.TrimSpace(str)
		if len(str) > 0 {
			structsArr = append(structsArr, "m."+str)
		}
	}

	p.Structs = structsArr
	p.InitParams = map[string]interface{}{
		"BackupDir": fmt.Sprintf(`"%s"`, "."),
	}
	err = t.Execute(fp, p)
	handleErr(err)

	cmd := exec.Command("go", "run", "transpiler.go")
	cmd.Dir = temDirpath
	fmt.Println(strings.Join(cmd.Args, " "))
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(output))
		handleErr(err)
	}
	fmt.Println(string(output))
}

func GetGolangFileStructs(filename string) ([]string, error) {
	fset := token.NewFileSet() // positions are relative to fset

	f, err := parser.ParseFile(fset, filename, nil, 0)
	if err != nil {
		return nil, err
	}

	v := &AVisitor{}
	ast.Walk(v, f)

	return v.structs, nil
}

type AVisitor struct {
	structNameCandidate string
	structs             []string
}

func (v *AVisitor) Visit(node ast.Node) ast.Visitor {
	if node != nil {
		switch t := node.(type) {
		case *ast.Ident:
			v.structNameCandidate = t.Name
		case *ast.StructType:
			if len(v.structNameCandidate) > 0 {
				v.structs = append(v.structs, v.structNameCandidate)
				v.structNameCandidate = ""
			}
		default:
			v.structNameCandidate = ""
		}
	}
	return v
}

func handleErr(err error) {
	if err != nil {
		panic(err.Error())
	}
}
