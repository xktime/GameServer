package utils

import (
	"context"
	"regexp"
)

var phoneNumberReg = "(\\d{3})\\d{4}(\\d{4})"

func MaskPhoneNumber(ctx context.Context, data string) string {
	reg, err := regexp.Compile(phoneNumberReg)
	if err != nil {
		return data
	}
	return reg.ReplaceAllString(data, "$1****$2")
}
