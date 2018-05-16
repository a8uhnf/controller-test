package handlers

import (
	"fmt"
	"reflect"
	api_v1 "k8s.io/api/core/v1"
)

type Logger struct{}

func (l *Logger) ObjectCreated(obj interface{}) {
	logEvent(l, obj, "created")
}

func (l *Logger) ObjectDeleted(obj interface{}) {
	logEvent(l, obj, "deleted")
}

func (l *Logger) ObjectUpdated(oldObj, newObj interface{}) {
	logEvent(l, newObj, "updated")
}

func logEvent(l *Logger, obj interface{}, ev string) {
	fmt.Println("__________Start__________")
	fmt.Printf("Resource %s: %s\n", reflect.TypeOf(obj), ev)
	fmt.Println(obj.(*api_v1.Secret).ObjectMeta.Name)
	fmt.Println("__________Finish_________")
}
