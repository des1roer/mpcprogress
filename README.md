# mpcprogress

Окно с прогрессом воспроизведения MPC-HC (сколько проиграно / сколько осталось),
данные берутся из встроенного веб-интерфейса MPC-HC (`variables.html`).

## Требования

- Windows
- В MPC-HC включён веб-интерфейс: **View → Options → Player → Web Interface**,
  поставить галку **Listen on port**, порт по умолчанию `7777`.
- Go 1.26+ и компилятор C (MinGW, нужен для сборки Fyne/cgo) — только если собираете из исходников.

## Быстрый запуск (готовый exe)

Скачайте `mpcprogress.exe` из релиза/репозитория и запустите:

```
mpcprogress.exe
```

По умолчанию программа опрашивает `http://localhost:7777/variables.html` раз в 500 мс.

## Настройка адреса через .env

Вместо флага URL можно задать его переменной окружения `MPC_URL` в файле `.env`
рядом с exe (или рядом с `main.go` при запуске через `go run`):

```
cp .env.example .env
```

Содержимое `.env`:

```
MPC_URL=http://localhost:7777/variables.html
```

Если `.env` отсутствует — используется значение по умолчанию
(`http://localhost:7777/variables.html`).

## Флаги командной строки

Флаг имеет приоритет над `.env` / значением по умолчанию:

```
mpcprogress.exe -url http://localhost:7777/variables.html -interval 500ms
```

- `-url` — адрес страницы `variables.html` MPC-HC
- `-interval` — интервал опроса (например `500ms`, `1s`)

## Сборка из исходников

```
go build -ldflags="-H=windowsgui" -o mpcprogress.exe .
```

Флаг `-ldflags="-H=windowsgui"` убирает консольное окно при запуске (PE-подсистема
переключается с Console на Windows GUI). Без него exe будет открывать помимо окна
ещё и чёрную консоль.

Если сборка на Windows падает с ошибкой про `go-gl`/`gl.v2.1` — значит не включён cgo
или не установлен компилятор C (поставьте MinGW, например `choco install mingw`).
