package actions

//admin:generate
//admin:action name=activate display="Activate User"
//admin:action name=purge destructive
type User struct {
	ID uint `json:"id" admin:"id"`
}
