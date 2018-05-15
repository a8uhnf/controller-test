package handlers

import (
	"fmt"
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
	fmt.Println("----------")
	fmt.Printf("Resource %s: %s", obj, ev)
}
