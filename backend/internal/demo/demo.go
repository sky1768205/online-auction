// Package demo описывает демо-аккаунты для быстрого входа без пароля.
// Пользователей создаёт cmd/seed, вход — POST /api/auth/demo (только при DEMO_LOGIN=true).
package demo

type Account struct {
	Role        string `json:"role"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DisplayName string `json:"displayName"`
	Email       string `json:"-"`
}

var Accounts = []Account{
	{
		Role:        "buyer",
		Title:       "Покупатель",
		Description: "Участвует в торгах: в одних лидирует, в других ставку перебили, один лот уже выиграл.",
		DisplayName: "Денис",
		Email:       "dan@example.test",
	},
	{
		Role:        "seller",
		Title:       "Продавец",
		Description: "Выставила несколько лотов: на одни идут ставки, один продан, один остался без покупателя.",
		DisplayName: "Алиса",
		Email:       "alice@example.test",
	},
	{
		Role:        "admin",
		Title:       "Администратор",
		Description: "Видит статистику и ленту ставок, может снять лот с торгов и заблокировать пользователя.",
		DisplayName: "Ольга",
		Email:       "admin@example.test",
	},
}

func Find(role string) (Account, bool) {
	for _, a := range Accounts {
		if a.Role == role {
			return a, true
		}
	}
	return Account{}, false
}
