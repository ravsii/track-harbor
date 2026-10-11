package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"meshvault/internal/db"
	"meshvault/internal/repository"
	"meshvault/internal/source/spotify"
)

var syncCmd = &cobra.Command{
	Use:          "sync",
	Short:        "Импортировать список треков из Spotify",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		started := time.Now()
		out := cmd.OutOrStdout()
		fmt.Fprintln(out, "[1/4] Проверяю сохранённые заголовки в ~/.meshvault/vault.db…")
		database, err := db.New()
		if err != nil {
			return fmt.Errorf("[1/4] открытие БД: %w", err)
		}
		defer database.Close()
		if err := database.Ping(); err != nil {
			return fmt.Errorf("[1/4] открытие БД: %w", err)
		}
		headerRepo := repository.NewSpotifyHeadersRepository(database)
		headers, err := headerRepo.Load()
		if err != nil {
			return fmt.Errorf("[1/4] чтение сохранённых заголовков: %w", err)
		}

		var info *spotify.PlaylistResponse
		if headers != nil {
			fmt.Fprintln(out, "[2/4] Запрашиваю первую страницу Spotify с сохранёнными заголовками…")
			info, err = spotify.GetSongInfo(headers.Authorization, headers.ClientToken)
			if err != nil {
				fmt.Fprintf(out, "      Запрос с сохранёнными заголовками не удался: %v\n", err)
			}
		} else {
			fmt.Fprintln(out, "      Сохранённых заголовков нет.")
		}
		if headers == nil || err != nil {
			fresh, err := readSyncHeaders(cmd.InOrStdin(), out)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "[2/4] Запрашиваю первую страницу Spotify с новыми заголовками…")
			info, err = spotify.GetSongInfo(fresh.Authorization, fresh.ClientToken)
			if err != nil {
				return fmt.Errorf("[2/4] запрос Spotify: %w", err)
			}
			if err := headerRepo.Save(*fresh); err != nil {
				return fmt.Errorf("[2/4] сохранение проверенных заголовков: %w", err)
			}
			fmt.Fprintln(out, "      Проверенные заголовки сохранены для следующего sync.")
			headers = fresh
		}

		playlist := info.Data.PlaylistV2
		total := playlist.Content.TotalCount
		fmt.Fprintf(out, "      Плейлист: %q.\n", playlist.Name)
		fmt.Fprintf(out, "      Страница 1 (offset=0): получено %d из %d треков.\n", len(playlist.Content.Items), total)
		if total < 0 || len(playlist.Content.Items) > total || len(playlist.Content.Items) == 0 && total > 0 {
			return fmt.Errorf("[2/4] первая страница содержит неполный или некорректный список треков")
		}
		for pageNumber := 2; len(playlist.Content.Items) < total; pageNumber++ {
			offset := len(playlist.Content.Items)
			fmt.Fprintf(out, "      Запрашиваю страницу %d (offset=%d)…\n", pageNumber, offset)
			next, err := spotify.GetSongInfoPage(headers.Authorization, headers.ClientToken, offset)
			if err != nil {
				return fmt.Errorf("[2/4] страница %d (offset=%d): %w", pageNumber, offset, err)
			}
			content := next.Data.PlaylistV2.Content
			if content.TotalCount != total {
				return fmt.Errorf("[2/4] количество треков изменилось во время загрузки; повторите sync")
			}
			if content.PagingInfo.Offset != offset {
				return fmt.Errorf("[2/4] страница %d: Spotify вернул неверное смещение %d вместо %d", pageNumber, content.PagingInfo.Offset, offset)
			}
			if len(content.Items) == 0 || len(content.Items) > total-offset {
				return fmt.Errorf("[2/4] страница %d: неполный или некорректный список треков", pageNumber)
			}
			playlist.Content.Items = append(playlist.Content.Items, content.Items...)
			fmt.Fprintf(out, "      Страница %d (offset=%d): получено %d из %d треков.\n", pageNumber, offset, len(playlist.Content.Items), total)
		}
		fmt.Fprintf(out, "      Получено %d из %d треков.\n", len(playlist.Content.Items), total)
		var ss []repository.Song
		unavailable := 0

		for i, song := range playlist.Content.Items {
			title := song.ItemV3.Data.IdentityTrait.Name
			artists := make([]string, 0, len(song.ItemV3.Data.IdentityTrait.Contributors.Items))

			for _, c := range song.ItemV3.Data.IdentityTrait.Contributors.Items {
				if c.Name != "" {
					artists = append(artists, c.Name)
				}
			}
			if title == "" {
				title = song.ItemV2.Data.Name
				artists = artists[:0]
				for _, artist := range song.ItemV2.Data.Artists.Items {
					if artist.Profile.Name != "" {
						artists = append(artists, artist.Profile.Name)
					}
				}
			}
			if strings.TrimSpace(title) == "" {
				unavailable++
				fmt.Fprintf(out, "      %d. Недоступный трек — пропущен.\n", i+1)
				continue
			}

			var buf bytes.Buffer

			err := json.NewEncoder(&buf).Encode(artists)
			if err != nil {
				return fmt.Errorf("[2/4] подготовка трека %d: %w", i+1, err)
			}
			artistName := strings.Join(artists, ", ")
			if artistName == "" {
				artistName = "Неизвестный исполнитель"
			}
			fmt.Fprintf(out, "      %d. %s — %s\n", i+1, artistName, title)

			ss = append(ss, repository.Song{
				Artists:   buf.String(),
				Title:     title,
				CreatedAt: time.Now(),
			})
		}

		fmt.Fprintln(out, "[3/4] Заменяю список треков в ~/.meshvault/vault.db…")
		repo := repository.NewSongRepository(database)
		saved, err := repo.ReplaceAll(ss)
		if err != nil {
			return fmt.Errorf("[3/4] сохранение треков (предыдущий список сохранён): %w", err)
		}
		fmt.Fprintf(out, "[4/4] Готово. Сохранено %d треков за %.1f с.\n", saved, time.Since(started).Seconds())
		if repeats := len(ss) - saved; repeats > 0 {
			fmt.Fprintf(out, "      Повторов: %d (сохранены по одному экземпляру).\n", repeats)
		}
		if unavailable > 0 {
			fmt.Fprintf(out, "      Пропущено недоступных треков: %d.\n", unavailable)
		}
		return nil
	},
}

func readSyncHeaders(input io.Reader, out io.Writer) (*repository.SpotifyHeaders, error) {
	fmt.Fprintln(out, "[1/4] Вставьте заголовки запроса Spotify, не cookies.")
	fmt.Fprintln(out, "      Формат (замените значения своими):")
	fmt.Fprintln(out, "      authorization: Bearer <токен>")
	fmt.Fprintln(out, "      client-token: <токен клиента>")
	fmt.Fprintln(out, "      Завершите ввод пустой строкой (Enter ещё раз) или Ctrl+D.")
	hMap, skipped, err := parseHeaders(input)
	if err != nil {
		return nil, fmt.Errorf("[1/4] чтение заголовков: %w", err)
	}
	fmt.Fprintf(out, "      Прочитано %d заголовков.\n", len(hMap))
	if skipped > 0 {
		fmt.Fprintf(out, "      Пропущено некорректных строк: %d.\n", skipped)
	}
	presence := func(value string) string {
		if value == "" {
			return "нет"
		}
		return "есть"
	}
	fmt.Fprintf(out, "      authorization: %s; client-token: %s\n", presence(hMap["authorization"]), presence(hMap["client-token"]))
	if hMap["authorization"] == "" || hMap["client-token"] == "" {
		return nil, fmt.Errorf("[1/4] необходимы непустые заголовки authorization и client-token из запроса Spotify Web Player; строка Cookie их не заменяет")
	}
	return &repository.SpotifyHeaders{Authorization: hMap["authorization"], ClientToken: hMap["client-token"]}, nil
}

func parseHeaders(input io.Reader) (map[string]string, int, error) {
	scanner := bufio.NewScanner(input)

	headerMap := make(map[string]string)
	skipped := 0

	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) == "" {
			break
		}
		headers := strings.SplitN(text, ":", 2)
		if len(headers) != 2 {
			skipped++
			continue
		}
		key, value := strings.ToLower(strings.TrimSpace(headers[0])), strings.TrimSpace(headers[1])
		// HTTP header names are ASCII tokens. In particular, a ':' inside
		// a cookie URL does not turn the cookie string into a header.
		invalidName := strings.IndexFunc(key, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r))
		}) >= 0
		if key == "" || invalidName {
			skipped++
			continue
		}
		headerMap[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}

	return headerMap, skipped, nil
}
