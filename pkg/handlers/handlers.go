package handlers

// Handler is implemented by any handler.
// The Handle method is used to process event
type Handler interface {
	ObjectCreated(obj interface{})
	ObjectDeleted(obj interface{})
	ObjectUpdated(oldObj, newObj interface{})
}
