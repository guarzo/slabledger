package inventory

import "reflect"

// DHDependenciesPresent checks configured integration boundaries, including
// typed-nil pointers stored in narrow ports. It never substitutes a no-op scope.
func DHDependenciesPresent(dependencies ...any) bool {
	for _, dependency := range dependencies {
		if dependency == nil {
			return false
		}
		value := reflect.ValueOf(dependency)
		switch value.Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Func:
			if value.IsNil() {
				return false
			}
		}
	}
	return true
}
