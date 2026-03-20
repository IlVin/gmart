package jserr

type JsError struct {
	Status int    `json:"-"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

func (e *JsError) Error() string {
	return e.Title
}

func (e *JsError) GetStatus() int {
	return e.Status
}

func (e *JsError) ContentType(ct string) string {
	return "application/json"
}
