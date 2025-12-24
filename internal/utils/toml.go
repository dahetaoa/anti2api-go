package utils

import (
	"strconv"
	"strings"
)

// ParseTOML 解析 TOML 格式字符串
func ParseTOML(input string) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	var currentArrayName string
	var currentArray []map[string]interface{}
	var currentObj map[string]interface{}

	lines := strings.Split(input, "\n")

	for _, rawLine := range lines {
		line := stripInlineComment(strings.TrimSpace(rawLine))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// [[table]] 数组
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			section := strings.TrimSpace(line[2 : len(line)-2])

			// 保存之前的对象
			if currentObj != nil && currentArrayName != "" {
				if result[currentArrayName] == nil {
					result[currentArrayName] = []map[string]interface{}{}
				}
				currentArray = result[currentArrayName].([]map[string]interface{})
				currentArray = append(currentArray, currentObj)
				result[currentArrayName] = currentArray
			}

			currentArrayName = section
			currentObj = make(map[string]interface{})
			continue
		}

		// [table] 单个表
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// 保存之前的对象
			if currentObj != nil && currentArrayName != "" {
				if result[currentArrayName] == nil {
					result[currentArrayName] = []map[string]interface{}{}
				}
				currentArray = result[currentArrayName].([]map[string]interface{})
				currentArray = append(currentArray, currentObj)
				result[currentArrayName] = currentArray
			}
			currentArrayName = ""
			currentObj = nil
			continue
		}

		// key = value
		if idx := strings.Index(line, "="); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := parseValue(strings.TrimSpace(line[idx+1:]))

			if currentObj != nil {
				currentObj[key] = value
			} else {
				result[key] = value
			}
		}
	}

	// 保存最后一个对象
	if currentObj != nil && currentArrayName != "" {
		if result[currentArrayName] == nil {
			result[currentArrayName] = []map[string]interface{}{}
		}
		currentArray = result[currentArrayName].([]map[string]interface{})
		currentArray = append(currentArray, currentObj)
		result[currentArrayName] = currentArray
	}

	return result, nil
}

func stripInlineComment(line string) string {
	// 查找不在引号内的 # 号
	inQuote := false
	for i, c := range line {
		if c == '"' {
			inQuote = !inQuote
		} else if c == '#' && !inQuote {
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}

func parseValue(raw string) interface{} {
	// 去除首尾空格
	raw = strings.TrimSpace(raw)

	// 字符串 "..."
	if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		return raw[1 : len(raw)-1]
	}

	// 字符串 '...'
	if strings.HasPrefix(raw, `'`) && strings.HasSuffix(raw, `'`) {
		return raw[1 : len(raw)-1]
	}

	// 布尔值
	if raw == "true" {
		return true
	}
	if raw == "false" {
		return false
	}

	// 整数
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i
	}

	// 浮点数
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}

	// 数组 [...]
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		return parseArray(raw[1 : len(raw)-1])
	}

	return raw
}

func parseArray(content string) []interface{} {
	var result []interface{}
	content = strings.TrimSpace(content)
	if content == "" {
		return result
	}

	// 简单解析，不处理嵌套
	parts := strings.Split(content, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, parseValue(p))
		}
	}
	return result
}
