package bad_page_size

//admin:generate default_page_size=abc
type User struct {
	ID uint `json:"id" admin:"id"`
}
