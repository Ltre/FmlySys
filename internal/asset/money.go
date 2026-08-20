package asset

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseYuan(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("金额不能为空")
	}
	neg := false
	if strings.HasPrefix(v, "-") {
		neg = true
		v = strings.TrimPrefix(v, "-")
	}
	parts := strings.Split(v, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("金额格式错误")
	}
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("金额格式错误")
	}
	cent := int64(0)
	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) > 2 {
			return 0, fmt.Errorf("金额最多两位小数")
		}
		if len(frac) == 1 {
			frac += "0"
		}
		if frac != "" {
			cent, err = strconv.ParseInt(frac, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("金额格式错误")
			}
		}
	}
	out := yuan*100 + cent
	if neg {
		out = -out
	}
	return out, nil
}

func FormatYuan(c int64) string {
	sign := ""
	if c < 0 {
		sign = "-"
		c = -c
	}
	return fmt.Sprintf("%s%d.%02d", sign, c/100, c%100)
}
