package config

// Часть кода в этом файле написана ИИ:
// Запрос: Имеется вложенная golang структура, каждом полю задан тег. Как посмотреть теги всех полей?
// https://chat.deepseek.com/a/chat/s/4d8221f2-fd41-4ad7-b1e6-d52ca5887fb5

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// GroupDef - атрибуты группы параметров.
type GroupDef struct {
	Items []Def `json:"items"`
}

// ItemDef - атрибуты параметра конфигурации.
type ItemDef struct {
	Type    string   `json:"type"`
	Default string   `json:"default,omitempty"`
	Options []string `json:"options,omitempty"`
}

// Def - атрибуты группы/параметра конфигурации.
type Def struct {
	Name        string    `json:"name"`
	Env         string    `json:"env,omitempty"`
	Description string    `json:"description,omitempty"`
	IsGroup     bool      `json:"is_group,omitempty"`
	Group       *GroupDef `json:"group,omitempty"`
	Item        *ItemDef  `json:"item,omitempty"`
}

var (
	// LineFormatMk - формат строк параметра для Makefile.
	LineFormatMk = "#- %s (%s) [%s]\n%-20s ?= %s\n"
	// HeaderFormatMk - формат строки названия группы параметров для Makefile.
	HeaderFormatMk = "\n# %s\n\n"

	// LineFormatMD - формат строки параметра для Markdown.
	LineFormatMD = "| %-20s | %-20s | %s | %s | %s |\n"
	// HeaderFormatMD - формат строки названия группы параметров для Markdown.
	HeaderFormatMD = "\n### %s%s\n\n"
	// TableHeaderMD - шапка таблицы группы параметров для Markdown.
	TableHeaderMD = `| Name | ENV | Type | Default | Description |` + "\n|------|-----|------|---------|-------------|"
)

// PrintConfig fetches config tags from obj struct and prints them in given format.
func PrintConfig(obj any, format string) {
	defs := FetchDefs(obj)
	// .md .mk .json
	if defs == nil {
		return
	}
	switch format {
	case "json":
		val, err := json.MarshalIndent(defs, "", "    ")
		if err != nil {
			//fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(val))
	case "md":
		PrintConfigM(defs, false, "", "", "Main Options")
	case "mk":
		PrintConfigM(defs, true, "", "", "Main Options")
	}

}

// PrintConfigM выводит конфиг в формате Makefile (onlyEnv) или MarkDown.
func PrintConfigM(defs []Def, onlyEnv bool, namePrefix, envPrefix, title string) {

	var fieldsFound bool
	childs := []Def{}
	for _, def := range defs {
		if onlyEnv && def.Env == "" {
			continue
		}
		if def.IsGroup {
			// Вложенные группы подключим позже
			childs = append(childs, def)
			continue
		}
		if !fieldsFound {
			// Время вывести заголовок группы
			fieldsFound = true
			if !onlyEnv {
				np := namePrefix
				if np != "" {
					np = " {#" + np + "}"
				}
				fmt.Printf(HeaderFormatMD, title, np)
				fmt.Println(TableHeaderMD)
			} else {
				fmt.Printf(HeaderFormatMk, title)
			}
			if namePrefix != "" {
				namePrefix = namePrefix + "."
			}
			if envPrefix != "" {
				envPrefix = envPrefix + "_"
			}
		}
		typ := def.Item.Type
		d := def.Item.Default
		if def.Item.Options != nil {
			// заменим тип на список допустимых вариантов
			typ = strings.Join(def.Item.Options, ",")
		} else if typ == "bool" && d == "" {
			// у bool по умолчанию - false
			d = "false"
		}
		n := namePrefix + def.Name
		e := envPrefix + def.Env
		if d != "" {
			d = "`" + d + "`"
		}
		if onlyEnv {
			fmt.Printf(LineFormatMk, def.Description, typ, d, e, def.Item.Default)
		} else {
			if def.Env == "" {
				e = "-"
			}
			de := strings.ReplaceAll(d, "\n", `\n`)
			fmt.Printf(LineFormatMD, n, e, typ, de, def.Description)
		}
	}
	for _, def := range childs {
		// Подключаем вложенные группы
		np := namePrefix + def.Name
		ep := envPrefix + def.Env
		PrintConfigM(def.Group.Items, onlyEnv, np, ep, def.Description)
	}
}

// FetchDefs fetch config definitions from Config struct.
func FetchDefs(obj any) []Def {

	v := reflect.ValueOf(obj)

	// Разыменовываем указатели
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Проверяем, что значение валидно и является структурой
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()

	rv := []Def{}
	for i := range t.NumField() {
		field := t.Field(i)
		fv := v.Field(i)
		ft := field.Type

		// Обрабатываем указатели на структуры
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		def := fetchFields(field.Tag)
		if def == nil {
			continue
		}
		if !def.IsGroup && ft.Kind() != reflect.Struct {
			def.Item.Type = ft.String()
			rv = append(rv, *def)
			continue
		}

		if fv.CanInterface() {
			// Пропускаем nil-указатели
			if fv.Kind() == reflect.Ptr && fv.IsNil() {
				continue
			}
			if def.IsGroup {
				def.Group = &GroupDef{Items: FetchDefs(fv.Interface())}
				rv = append(rv, *def)
			} else {
				siblings := FetchDefs(fv.Interface())
				rv = append(rv, siblings...)
			}
		}

	}
	return rv
}

// Компилируем регулярное выражение для поиска значений в choice:"..."
var reOptions = regexp.MustCompile(`choice:"([^"]*)"`)

// Список тегов, поддерживаемых https://github.com/jessevdk/go-flags/
var tagFields = []string{"hidden", "env", "default", "long", "choice", "description", "group", "namespace", "env-namespace", "positional-arg-name"}

// Извлечение поддерживаемых тегов
func fetchFields(tag reflect.StructTag) *Def {
	rv := map[string]string{}
	for _, field := range tagFields {
		val, ok := tag.Lookup(field)
		if ok {
			if field == "hidden" && val != "" {
				return nil
			}
			rv[field] = val
		}
	}
	if rv["group"] != "" {
		return &Def{
			Name:        rv["namespace"],
			Env:         rv["env-namespace"],
			Description: rv["group"],
			IsGroup:     true,
		}
	}
	var result []string
	// Ищем все choice в строке
	matches := reOptions.FindAllStringSubmatch(string(tag), -1)
	if len(matches) > 0 {
		// choice:"off" choice:"server" choice:"client"

		// Извлекаем значения из групп
		result = make([]string, 0, len(matches))
		for _, match := range matches {
			if len(match) >= 2 { // Проверяем наличие группы
				result = append(result, match[1])
			}
		}
	}

	def := Def{
		Name:        rv["long"],
		Env:         rv["env"],
		Description: rv["description"],
		Item: &ItemDef{
			Default: rv["default"],
			Options: result,
		},
	}
	if def.Name == "" {
		def.Name = rv["positional-arg-name"]
	}
	return &def
}
