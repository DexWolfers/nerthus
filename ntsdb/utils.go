// zwj@2017-09-29

// 数据库接口

package ntsdb

import (
	"fmt"
	"reflect"
	"strconv"
)

// 将数据映射到结构体中
func structFieldMap(data reflect.Value, refs []interface{}) error {
	num := len(refs)
	var strdata string
	for i := 0; i < num; i++ {
		fv := data.Field(i)
		if !fv.CanSet() {
			continue
		}

		val := reflect.ValueOf(refs[i]).Elem().Interface()
		switch val.(type) {
		case int, int64:
			strdata = fmt.Sprintf("%d", val.(int64))
		case int32:
			strdata = fmt.Sprintf("%d", val.(int32))
		case []uint8:
			strdata = fmt.Sprintf("%s", val.([]uint8))
		case string:
			strdata = val.(string)
		case float32:
			strdata = fmt.Sprintf("%f", val.(float32))
		case float64:
			strdata = fmt.Sprintf("%f", val.(float64))
		}

		switch fv.Type().Kind() {
		case reflect.String:
			fv.SetString(strdata)
		case reflect.Int:
			intVal, _ := strconv.ParseInt(strdata, 10, 64)
			fv.SetInt(intVal)
		case reflect.Float32:
			fallthrough
		case reflect.Float64:
			floatVal, _ := strconv.ParseFloat(strdata, 32/64)
			fv.SetFloat(floatVal)
		case reflect.Slice:
			fv.SetBytes([]byte(strdata))
		default:
			return fmt.Errorf("undefine refelect type:%v", fv.Type().Kind())
		}

	}
	return nil
}
