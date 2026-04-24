package basic

//admin:generate
type User struct {
	ID   uint   `json:"id" admin:"id"`
	Name string `json:"name" admin:"filterable"`
}
