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

type ConfigGroupDef struct {
	Items []ConfigDef `json:"items"`
}
type ConfigItemDef struct {
	Type    string   `json:"type"`
	Default string   `json:"default,omitempty"`
	Options []string `json:"options,omitempty"`
}

type ConfigDef struct {
	Name        string          `json:"name"`
	Env         string          `json:"env,omitempty"`
	Description string          `json:"description,omitempty"`
	IsGroup     bool            `json:"is_group,omitempty"`
	Group       *ConfigGroupDef `json:"group,omitempty"`
	Item        *ConfigItemDef  `json:"item,omitempty"`
}

// Рекурсивная функция для вывода тегов полей структуры
func PrintConfig(obj interface{}, format string) {
	defs := FetchConfigDefs(obj)
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
		fmt.Println("| Name | ENV | Type | Default | Description |")
		fmt.Println("|------|-----|------|---------|-------------|")
		PrintConfigM(defs, false, "", "", "Main Options")
	case "mk":
		PrintConfigM(defs, true, "", "", "Main Options")
	}

}

// PrintConfigM выводит конфиг в формате Makefile (onlyEnv) или MarkDown
func PrintConfigM(defs []ConfigDef, onlyEnv bool, namePrefix, envPrefix, title string) {

	var fieldsFound bool
	childs := []ConfigDef{}
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
				fmt.Printf("| ## %s%s |\n", title, np)

			} else {
				fmt.Printf("\n# %s\n\n", title)
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
			d = fmt.Sprintf(" [%s]", d)
		} else {
			d = "-"
		}
		if onlyEnv {
			fmt.Printf("#- %s (%s)%s\n%-20s ?= %s\n",
				def.Description, typ, d, e, def.Item.Default)
		} else {
			if def.Env == "" {
				e = "-"
			}
			fmt.Printf("| %-20s | %-20s | %s | %s | %s |\n", n, e,
				typ, d, def.Description)
		}
	}
	for _, def := range childs {
		// Подключаем вложенные группы
		np := namePrefix + def.Name
		ep := envPrefix + def.Env
		PrintConfigM(def.Group.Items, onlyEnv, np, ep, def.Description)
	}
}

func FetchConfigDefs(obj interface{}) []ConfigDef {

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

	rv := []ConfigDef{}
	for i := 0; i < t.NumField(); i++ {
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
			def.Item.Type = ft.Name()
			rv = append(rv, *def)
			continue
		}

		if fv.CanInterface() {
			// Пропускаем nil-указатели
			if fv.Kind() == reflect.Ptr && fv.IsNil() {
				continue
			}
			if def.IsGroup {
				def.Group = &ConfigGroupDef{Items: FetchConfigDefs(fv.Interface())}
				rv = append(rv, *def)
			} else {
				siblings := FetchConfigDefs(fv.Interface())
				rv = append(rv, siblings...)
			}
		}

	}
	return rv
}

// Компилируем регулярное выражение для поиска значений в choice:"..."
var reOptions = regexp.MustCompile(`choice:"([^"]*)"`)

// Список тегов, поддерживаемых https://github.com/jessevdk/go-flags/
var tagFields = []string{"hidden", "env", "default", "long", "choice", "description", "group", "namespace", "env-namespace"}

// Извлечение поддерживаемых тегов
func fetchFields(tag reflect.StructTag) *ConfigDef {
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
		return &ConfigDef{
			Name:        rv["namespace"],
			Env:         rv["env-namespace"],
			Description: rv["group"],
			IsGroup:     true,
		}
	}
	var result []string
	if rv["choice"] != "" {
		// choice:"off"     choice:"server"                                choice:"client"

		// Ищем все совпадения в строке
		matches := reOptions.FindAllStringSubmatch(string(tag), -1)

		// Извлекаем значения из групп
		result = make([]string, 0, len(matches))
		for _, match := range matches {
			if len(match) >= 2 { // Проверяем наличие группы
				result = append(result, match[1])
			}
		}
	}
	return &ConfigDef{
		Name:        rv["long"],
		Env:         rv["env"],
		Description: rv["description"],
		Item: &ConfigItemDef{
			Default: rv["default"],
			Options: result,
		},
	}
}
