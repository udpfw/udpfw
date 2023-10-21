package config

import (
	"fmt"
	"github.com/urfave/cli/v2"
	"reflect"
	"strings"
)

var stringPtr = reflect.TypeOf((*string)(nil))
var filePathPtr = reflect.TypeOf((*FilePath)(nil))
var boolPtr = reflect.TypeOf((*bool)(nil))

func getField(v reflect.StructField, name string) (string, error) {
	value, ok := v.Tag.Lookup(name)
	if !ok {
		return "", fmt.Errorf("BUG: AllOptions field %s lacks tag %s", v.Name, name)
	}

	return value, nil
}

func envNamed(name string) []string {
	name = strings.ToUpper(name)
	return []string{"UDPFW_DISPATCH_" + name, "DISPATCH_" + name}
}

func makeCLIFlagsFrom[T any]() ([]cli.Flag, error) {
	t := reflect.TypeOf(*new(T))
	var opts []cli.Flag

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, ok := field.Tag.Lookup("name")
		if !ok {
			continue
		}

		env, err := getField(field, "env")
		if err != nil {
			return nil, err
		}

		usage, err := getField(field, "usage")
		if err != nil {
			return nil, err
		}

		category, _ := field.Tag.Lookup("category")
		value, _ := field.Tag.Lookup("value")

		var arg cli.Flag

		if field.Type == boolPtr || field.Type.Kind() == reflect.Bool {
			arg = &cli.BoolFlag{
				Name:     name,
				Category: category,
				Usage:    usage,
				EnvVars:  envNamed(env),
				Required: field.Type.Kind() == reflect.String,
				Value:    value == "true",
			}
		} else {
			strArg := &cli.StringFlag{
				Name:        name,
				Category:    category,
				Usage:       usage,
				EnvVars:     envNamed(env),
				Required:    field.Type.Kind() == reflect.String,
				DefaultText: value,
				Value:       value,
			}

			strArg.TakesFile = field.Type == filePathPtr
			arg = strArg
		}
		opts = append(opts, arg)
	}

	return opts, nil
}

func MakeCLIFlags() ([]cli.Flag, error) { return makeCLIFlagsFrom[AllOptions]() }

func OptionsFrom(ctx *cli.Context) (AllOptions, error) {
	rawV := &AllOptions{}
	t := reflect.TypeOf(rawV).Elem()
	v := reflect.ValueOf(rawV).Elem()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		targetField := v.Field(i)

		name, ok := field.Tag.Lookup("name")
		if !ok {
			continue
		}

		defaultValue, hasDefault := field.Tag.Lookup("value")
		if !ctx.IsSet(name) && !hasDefault {
			continue
		}

		var value reflect.Value
		if isString(targetField.Type()) {
			rawValue := ctx.String(name)
			if hasDefault && rawValue == "" {
				rawValue = defaultValue
			}
			if targetField.Type() == filePathPtr {
				path := FilePath(rawValue)
				value = reflect.ValueOf(&path)
			} else if targetField.Type() == stringPtr {
				value = reflect.ValueOf(&rawValue)
			} else if targetField.Kind() == reflect.String {
				value = reflect.ValueOf(rawValue)
			}
		} else if isBool(targetField.Type()) {
			rawValue := ctx.Bool(name)
			if targetField.Type() == boolPtr {
				value = reflect.ValueOf(&rawValue)
			} else if targetField.Kind() == reflect.Bool {
				value = reflect.ValueOf(rawValue)
			}
		} else {
			panic("Not implemented")
		}
		targetField.Set(value)

	}
	return *rawV, nil
}

func isString(t reflect.Type) bool {
	return t == stringPtr ||
		t.Kind() == reflect.String ||
		t == filePathPtr
}

func isBool(t reflect.Type) bool {
	return t == boolPtr ||
		t.Kind() == reflect.Bool
}
