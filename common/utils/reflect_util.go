package utils

import (
	"fmt"
	"reflect"
)

func CallMethodWithParams(instance interface{}, methodName string, params ...interface{}) ([]reflect.Value, error) {
	methodValue, err := GetMethod(instance, methodName)
	if err != nil {
		return nil, err
	}
	methodType := methodValue.Type()
	// 检查参数数量
	if len(params) != methodType.NumIn() {
		return nil, fmt.Errorf("参数数量不匹配: 期望 %d, 实际 %d",
			methodType.NumIn(), len(params))
	}

	// 准备反射参数切片
	in := make([]reflect.Value, len(params))

	// 转换并验证参数类型
	for i, param := range params {
		expectedType := methodType.In(i)
		if param == nil {
			// 允许 nil 传递给 interface 或指针类型
			if expectedType.Kind() == reflect.Interface || expectedType.Kind() == reflect.Ptr || expectedType.Kind() == reflect.Slice || expectedType.Kind() == reflect.Map || expectedType.Kind() == reflect.Func || expectedType.Kind() == reflect.Chan {
				in[i] = reflect.Zero(expectedType)
				continue
			} else {
				return nil, fmt.Errorf("参数 %d 不能为 nil, 期望类型: %v", i+1, expectedType)
			}
		}
		paramValue := reflect.ValueOf(param)
		paramType := paramValue.Type()
		// 检查类型是否可赋值
		if !paramType.AssignableTo(expectedType) {
			// 尝试转换数值类型
			if paramType.ConvertibleTo(expectedType) {
				converted := paramValue.Convert(expectedType)
				in[i] = converted
				continue
			}
			return nil, fmt.Errorf("参数 %d 类型不匹配: 期望 %v, 实际 %v",
				i+1, expectedType, paramType)
		}
		in[i] = paramValue
	}

	// 调用方法
	results := methodValue.Call(in)

	return results, nil
}

func GetMethod(instance interface{}, methodName string) (reflect.Value, error) {
	// 查找方法
	val := reflect.ValueOf(instance)
	method := val.MethodByName(methodName)
	if !method.IsValid() {
		if val.Kind() != reflect.Ptr {
			ptr := reflect.New(reflect.TypeOf(instance))
			ptr.Elem().Set(val)
			val = ptr
		}
		method = val.MethodByName(methodName)
		if !method.IsValid() {
			return reflect.Value{}, fmt.Errorf("method %s not found", methodName)
		}
	}
	return method, nil
}
