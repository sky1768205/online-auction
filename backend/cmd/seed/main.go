// Команда seed заполняет базу демо-данными: пользователи, лоты, ставки.
//
//	go run ./cmd/seed          — добавить данные
//	go run ./cmd/seed -reset   — сначала очистить пользователей, лоты и ставки
//
// Демо-аккаунты (только для локальной разработки): пароль у всех — demoPassword ниже.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"lot/internal/closer"
	"lot/internal/config"
	"lot/internal/db"
)

const demoPassword = "demo-password"

// Алиса, Денис и Ольга — демо-аккаунты для быстрого входа (см. internal/demo):
// Алиса в основном продаёт, Денис в основном покупает, Ольга — администратор.
// Борис и Катя — фоновые участники торгов.
var demoUsers = []struct{ email, name string }{
	{"alice@example.test", "Алиса"},
	{"bob@example.test", "Борис"},
	{"carol@example.test", "Катя"},
	{"dan@example.test", "Денис"},
}

// Администраторы создаются отдельно: в торгах не участвуют.
var demoAdmins = []struct{ email, name string }{
	{"admin@example.test", "Ольга"},
}

type demoLot struct {
	seller   int
	category string
	title    string
	desc     string
	start    int64 // рубли
	step     int64 // рубли
	duration time.Duration
	bidders  []int    // индексы пользователей, делавших ставки по очереди
	images   []string // фотографии с Wikimedia Commons (свободные лицензии, авторы — в README)
}

const (
	wmThumb  = "https://thumb.wikimedia.org/wikipedia/commons/thumb/"
	wmUpload = "https://upload.wikimedia.org/wikipedia/commons/"
)

var demoLots = []demoLot{
	{0, "electronics", "Плёночный фотоаппарат «Зенит-Е»",
		"Классическая советская зеркальная камера с объективом Гелиос-44-2 58 мм f/2. Механика в порядке, затвор отрабатывает все выдержки, селеновый экспонометр живой. Оригинальный кожаный футляр.",
		3000, 200, 15 * time.Minute, []int{1, 3, 2}, []string{
			wmThumb + "6/63/Zenit-E_mit_Helios-44-2_--_2023_--_0024-31.jpg/1920px-Zenit-E_mit_Helios-44-2_--_2023_--_0024-31.jpg",
			wmThumb + "5/52/2024_Zenit-E_%282%29.jpg/1920px-2024_Zenit-E_%282%29.jpg",
			wmThumb + "5/55/2024_Zenit-E_%286%29.jpg/1920px-2024_Zenit-E_%286%29.jpg",
		}},
	{1, "art", "Копия картины И. И. Шишкина «Зима»",
		"Авторская копия, холст, масло, 60×100 см. Выполнена выпускником Академии художеств в 1970-х годах. В багетной раме, состояние хорошее.",
		15000, 1000, 2 * time.Hour, []int{2, 3}, []string{
			wmUpload + "d/d1/Ivan_Shishkin_-_Winter.JPG",
		}},
	{2, "collectibles", "Значок «Москва-80»",
		"Памятный значок Олимпийских игр 1980 года. Эмаль без сколов, застёжка родная.",
		500, 50, 6 * time.Hour, nil, []string{
			wmThumb + "a/a9/80_%D0%9C%D0%BE%D1%81%D0%BA%D0%B2%D0%B0_%D0%B7%D0%BD%D0%B0%D1%87%D0%BE%D0%BA.jpg/1920px-80_%D0%9C%D0%BE%D1%81%D0%BA%D0%B2%D0%B0_%D0%B7%D0%BD%D0%B0%D1%87%D0%BE%D0%BA.jpg",
		}},
	{0, "watches", "Хронограф «Полёт Буран», около 1990 года",
		"Механический хронограф на калибре 3133, выпущен в честь программы «Буран». Корпус из нержавеющей стали, после обслуживания у мастера, ход точный.",
		18000, 1000, 1 * 24 * time.Hour, []int{1, 3, 1, 3}, []string{
			wmThumb + "6/6c/Poljot_Buran_chronograph_watch_from_about_1990.jpg/1920px-Poljot_Buran_chronograph_watch_from_about_1990.jpg",
			wmUpload + "b/bd/Poljot_Buran_chronograph_watch_from_about_1990_%28detail%29.jpg",
		}},
	{2, "books", "А. П. Чехов. Сочинения, дореволюционное издание",
		"Том первый (1879–1887). Твёрдый переплёт, страницы без пятен и надписей. Редкое издание для коллекционеров русской литературы.",
		5000, 250, 3 * 24 * time.Hour, []int{3, 1}, []string{
			wmUpload + "f/fe/Chekov_works_vol_1.jpg",
			wmUpload + "e/e7/Anton_Chekhov_%28Plays%2C_1912_edition%29.png",
		}},
	{0, "home", "Чайный сервиз ЛФЗ «Кобальтовая сетка»",
		"Ленинградский фарфоровый завод, знаменитый рисунок Анны Яцкевич. Сервиз на 6 персон: чайник, сахарница, молочник, чашки и блюдца. Без сколов и трещин.",
		12000, 500, 2 * 24 * time.Hour, []int{2, 1}, []string{
			wmThumb + "5/57/Lomonosov-porcelain_hg_01.jpg/1920px-Lomonosov-porcelain_hg_01.jpg",
			wmUpload + "3/33/LFZ_chinaware_3.jpg",
			wmThumb + "0/0c/Lomonosov-porcelain_hg_04.jpg/1920px-Lomonosov-porcelain_hg_04.jpg",
		}},
	{2, "electronics", "Катушечный магнитофон «Маяк-203»",
		"Стереофонический магнитофон первого класса, Киев. Полностью исправен, прошёл профилактику. В комплекте 5 катушек с записями.",
		20000, 1000, 5 * 24 * time.Hour, nil, []string{
			wmUpload + "c/c1/Majak203.jpg",
			wmUpload + "b/b2/Majak_201.jpg",
		}},
	{1, "art", "Гравюра «Вид Академии наук» по рисунку М. Махаева",
		"Санкт-Петербург, середина XVIII века, по рисунку Михаила Махаева. Лист 50×70 см, в паспарту. Вторая иллюстрация — «Невский проспект и Адмиралтейство» из той же серии для сравнения.",
		7000, 300, 25 * time.Minute, []int{3}, []string{
			wmThumb + "c/c2/SPb_Academy_of_Sciences_Kachalov_dr_Mahaev_1753.jpg/1920px-SPb_Academy_of_Sciences_Kachalov_dr_Mahaev_1753.jpg",
			wmThumb + "8/85/Nevskii_prospect_and_Admiralty.jpg/1920px-Nevskii_prospect_and_Admiralty.jpg",
		}},
	{2, "collectibles", "Банкнота 10 000 рублей 1921 года",
		"Расчётный знак РСФСР. Состояние VF, без надрывов. Яркий оборот с орнаментом.",
		3000, 200, 12 * time.Hour, []int{1, 3, 1, 3, 1}, []string{
			wmThumb + "9/9b/10000_roubles_Soviet_Union%2C_1921_back.jpg/1920px-10000_roubles_Soviet_Union%2C_1921_back.jpg",
			wmThumb + "e/e0/10000_roubles_Soviet_Union%2C_1921_Front.jpg/1920px-10000_roubles_Soviet_Union%2C_1921_Front.jpg",
		}},
	{0, "watches", "Карманные часы Omega, серебро",
		"Швейцария, начало XX века. Серебряный корпус, эмалевый циферблат с секундной стрелкой. Ход точный, на цепочке.",
		45000, 2500, 4 * 24 * time.Hour, nil, []string{
			wmThumb + "9/94/Omega_Pocket_Watch.jpg/1920px-Omega_Pocket_Watch.jpg",
		}},
	// Уже завершённые торги (ends_at в прошлом) — закрываются сразу после сида.
	{0, "books", "Факсимиле Каталонского атласа 1375 года",
		"Факсимильное издание знаменитой средневековой карты мира Авраама Крескеса. Большой формат.",
		1500, 100, -2 * time.Hour, []int{2, 3}, []string{
			wmThumb + "3/35/1375_Atlas_Catalan_Abraham_Cresques.jpg/1920px-1375_Atlas_Catalan_Abraham_Cresques.jpg",
		}},
	{0, "home", "Настольная лампа с зелёным абажуром",
		"Классическая «банкирская» лампа: зелёное стекло, латунное основание. Работает.",
		4000, 200, -30 * time.Minute, nil, []string{
			wmThumb + "0/08/Bankers_lamp_by_Rob_Pongsajapan.jpg/1920px-Bankers_lamp_by_Rob_Pongsajapan.jpg",
			wmThumb + "3/3f/Bankers_lamp_%2810806062373%29.jpg/1920px-Bankers_lamp_%2810806062373%29.jpg",
		}},
}

func main() {
	reset := flag.Bool("reset", false, "очистить данные перед заполнением")
	flag.Parse()

	ctx := context.Background()
	cfg := config.Load()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, slog.Default()); err != nil {
		log.Fatal(err)
	}

	if *reset {
		if _, err := pool.Exec(ctx, `TRUNCATE bids, lot_images, lots, users RESTART IDENTITY CASCADE`); err != nil {
			log.Fatal(err)
		}
	}

	hash, _ := bcrypt.GenerateFromPassword([]byte(demoPassword), bcrypt.DefaultCost)
	for _, a := range demoAdmins {
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (email, password_hash, display_name, role) VALUES ($1, $2, $3, 'admin')
			ON CONFLICT (email) DO UPDATE SET display_name = EXCLUDED.display_name, role = 'admin', blocked_at = NULL`,
			a.email, string(hash), a.name); err != nil {
			log.Fatal(err)
		}
	}

	userIDs := make([]int64, len(demoUsers))
	for i, u := range demoUsers {
		err := pool.QueryRow(ctx, `
			INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3)
			ON CONFLICT (email) DO UPDATE SET display_name = EXCLUDED.display_name, blocked_at = NULL
			RETURNING id`, u.email, string(hash), u.name).Scan(&userIDs[i])
		if err != nil {
			log.Fatal(err)
		}
	}

	now := time.Now()
	for _, d := range demoLots {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			endsAt := now.Add(d.duration)
			startsAt := now.Add(-time.Hour)
			if d.duration < 0 {
				startsAt = endsAt.Add(-24 * time.Hour)
			}

			var lotID int64
			if err := tx.QueryRow(ctx, `
				INSERT INTO lots (seller_id, category_id, title, description, start_price, min_step, starts_at, ends_at)
				SELECT $1, c.id, $3, $4, $5, $6, $7, $8 FROM categories c WHERE c.slug = $2
				RETURNING id`,
				userIDs[d.seller], d.category, d.title, d.desc, d.start*100, d.step*100, startsAt, endsAt,
			).Scan(&lotID); err != nil {
				return err
			}

			for n, url := range d.images {
				if _, err := tx.Exec(ctx,
					`INSERT INTO lot_images (lot_id, url, position) VALUES ($1, $2, $3)`, lotID, url, n,
				); err != nil {
					return err
				}
			}

			// Ставки идут с шагом от минимального до тройного, по времени — равномерно до «сейчас».
			price := d.start * 100
			bidTime := startsAt.Add(10 * time.Minute)
			for n, b := range d.bidders {
				if n > 0 {
					price += d.step * 100 * int64(1+n%3)
				}
				if _, err := tx.Exec(ctx,
					`INSERT INTO bids (lot_id, user_id, amount, created_at) VALUES ($1, $2, $3, $4)`,
					lotID, userIDs[b], price, bidTime,
				); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `
					UPDATE lots SET current_price = $2, leader_id = $3, bid_count = bid_count + 1 WHERE id = $1`,
					lotID, price, userIDs[b],
				); err != nil {
					return err
				}
				bidTime = bidTime.Add(7 * time.Minute)
			}
			return nil
		})
		if err != nil {
			log.Fatalf("lot %q: %v", d.title, err)
		}
	}

	closed, err := closer.CloseDue(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Готово: %d пользователей + %d администратор, %d лотов (%d уже завершены)\n",
		len(demoUsers), len(demoAdmins), len(demoLots), len(closed))
}
