package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

func main() {
	filename := "tartiflette.recipy.hcl"
	src, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("failed to open %s: %v", filename, err)
	}
	os.Exit(retMain(src, filename))
}

var recipyVersionSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name:     "recipy_version",
			Required: false,
		},
	},
}

var preparationActionsSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "slice", LabelNames: []string{"ingredient"}},
		{Type: "boil", LabelNames: []string{"ingredient"}},
	},
}

var stackContentSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "add"},
	},
}

var stackSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "stack", LabelNames: []string{"name"}},
	},
}

type Action struct {
	Duration string `hcl:"duration"`
	Verb     string
	What     string
}

func (a Action) String() string {
	s := fmt.Sprintf("%s %s", a.Verb, a.What)
	if a.Duration != "" {
		s += fmt.Sprintf(" during %s", a.Duration)
	}
	return s
}

var HCLMinutesFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name:         "minutes",
			Type:         cty.Number,
			AllowNull:    false,
			AllowUnknown: false,
		},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		minutes, _ := args[0].AsBigFloat().Int64()
		d := time.Duration(minutes) * time.Minute
		return cty.StringVal(d.String()), nil
	},
})

var durationEvalCtx = &hcl.EvalContext{
	Functions: map[string]function.Function{
		"minutes": HCLMinutesFunc,
	},
	Variables: map[string]cty.Value{
		"my_var": cty.StringVal("my_val"),
	},
}

func retMain(bytes []byte, filename string) int {
	file, diags := hclsyntax.ParseConfig(bytes, filename, hcl.Pos{Byte: 0, Line: 1, Column: 1})
	if diags.HasErrors() {
		log.Printf("%v", diags)
		return 1
	}
	files := map[string]*hcl.File{
		filename: file,
	}

	versionContent, rest, diags := file.Body.PartialContent(recipyVersionSchema)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}
	v, found := versionContent.Attributes["recipy_version"]
	if found {
		v, diags := v.Expr.Value(nil)
		if diags.HasErrors() {
			return writeDiags(files, diags)
		}
		fmt.Printf("expecting recipy version %s\n\n", v.AsString())
	}

	preparationContent, prepRest, diags := rest.PartialContent(preparationActionsSchema)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}

	var actions []Action
	for _, block := range preparationContent.Blocks {
		action := Action{
			Verb: block.Type,
			What: block.Labels[0],
		}
		switch block.Type {
		case "slice":
			// nothing to do here since this is empty
		case "boil":
			boilBody := block.Body
			diags := gohcl.DecodeBody(boilBody, durationEvalCtx, &action)
			if diags.HasErrors() {
				return writeDiags(files, diags)
			}
		}
		actions = append(actions, action)
	}

	stackContent, moreDiags := prepRest.Content(stackSchema)
	diags = append(diags, moreDiags...)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}

	var stack struct {
		What string
		In   string   `hcl:"in,optional"`
		Rest hcl.Body `hcl:",remain"`
	}
	for _, block := range stackContent.Blocks {
		switch block.Type {
		case "stack":
			stackBody := block.Body
			diags := gohcl.DecodeBody(stackBody, nil, &stack)
			if diags.HasErrors() {
				return writeDiags(files, diags)
			}
			stack.What = block.Labels[0]

			break // there can only be one stack here and we don't enforce it
		}
	}

	nestedStackContent, moreDiags := stack.Rest.Content(stackContentSchema)
	diags = append(diags, moreDiags...)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}
	stackAddsRequire := []string{}
	for _, block := range nestedStackContent.Blocks {
		switch block.Type {
		case "add":
			var stackSpec = &hcldec.ObjectSpec{
				"what":     &hcldec.AttrSpec{Name: "what", Type: cty.String, Required: true},
				"quantity": &hcldec.AttrSpec{Name: "quantity", Type: cty.String},
			}

			v, diags := hcldec.Decode(block.Body, stackSpec, nil)
			_ = v
			_ = diags

			// tell what is required by this block :
			traversals := hcldec.Variables(block.Body, stackSpec)
			for _, traversal := range traversals {
				split := traversal.SimpleSplit()
				// split.RootName() == boiled_potatoes or sliced_cheese
				stackAddsRequire = append(stackAddsRequire, split.RootName())
			}
		}
	}

	fmt.Printf("To prepare a %s:\n", stack.What)

	for i := range stackAddsRequire {
		action := actions[i]
		fmt.Printf("* %s\n", action)
	}

	fmt.Printf("\nThen stack in a %s\n\n", stack.In)

	fmt.Printf("tartiflette requires: %v\n\n", stackAddsRequire)

	return 0
}

func writeDiags(files map[string]*hcl.File, diags hcl.Diagnostics) int {
	// write HCL errors/diagnostics if any.
	b := bytes.NewBuffer(nil)
	err := hcl.NewDiagnosticTextWriter(b, files, 80, false).WriteDiagnostics(diags)
	if err != nil {
		log.Fatalf("could not write diagnostic: %v", err)
		return 1
	}
	if b.Len() != 0 {
		if diags.HasErrors() {
			log.Fatal(b.String())
			return 1
		}
		log.Print(b.String())
	}
	return 0
}
