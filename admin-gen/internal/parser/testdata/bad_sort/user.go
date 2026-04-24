package bad_sort

//admin:generate default_sort="name:sideways"
type User struct {
	ID uint `json:"id" admin:"id"`
}
