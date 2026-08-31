package activity

import "reflect"

func fieldExists(v any, name string) bool {
	_, ok := reflect.TypeOf(v).FieldByName(name)
	return ok
}
