# Библиотеки и источники данных

## Сервер

| Библиотека | Версия | Назначение | Адрес |
|---|---|---|---|
| Go | 1.27.1 | Язык сервера и ядра расчёта | https://go.dev |
| chi | 5.3.2 | HTTP-маршрутизатор | https://github.com/go-chi/chi |
| pgx | 5.11.0 | Драйвер и пул соединений Postgres | https://github.com/jackc/pgx |
| sqlc | нет | Генерация кода запросов к Postgres | https://sqlc.dev |
| golang-migrate | 4.20.1 | Применение миграций | https://github.com/golang-migrate/migrate |
| excelize | 2.11.0 | Чтение и запись файлов XLSX | https://github.com/xuri/excelize |
| fpdf | 0.9.0 | Формирование отчётов PDF | https://github.com/go-pdf/fpdf |
| uuid | 1.6.0 | Идентификаторы | https://github.com/google/uuid |
| golang.org/x/crypto | 0.57.0 | Хэширование паролей (bcrypt) | https://pkg.go.dev/golang.org/x/crypto |

## Веб-приложение

| Библиотека | Версия | Назначение | Адрес |
|---|---|---|---|
| React | 19 | Интерфейс | https://react.dev |
| React Router | 7 | Маршрутизация | https://reactrouter.com |
| Vite | 8 | Сборка | https://vite.dev |
| TypeScript | 7 | Язык интерфейса | https://www.typescriptlang.org |
| TanStack Query | 5 | Запросы к API и кэш | https://tanstack.com/query |
| Konva и react-konva | 10 и 19 | Редактор карты и воспроизведение симуляции | https://konvajs.org |
| pdf.js | 6 | Чтение файла плана в формате PDF | https://mozilla.github.io/pdf.js |
| SheetJS (сборка `@e965/xlsx`) | 0.20.3 | Чтение шаблонов параметров XLSX и CSV в браузере | https://sheetjs.com |
| IBM Plex Sans, IBM Plex Mono | нет | Шрифты (лицензия OFL) | https://github.com/IBM/plex |
| Tabler Icons | нет | Значки (лицензия MIT), скопированы в `web/src/ui/icons.tsx` | https://tabler.io/icons |

## Инструменты разработки

| Инструмент | Назначение | Адрес |
|---|---|---|
| Docker и Docker Compose | Запуск платформы | https://docs.docker.com |
| Postgres 18 | База данных | https://www.postgresql.org |
| nginx | Раздача приложения и проксирование API | https://nginx.org |
| Playwright | Сквозные тесты | https://playwright.dev |
| Vitest | Тесты интерфейса | https://vitest.dev |
| oxlint | Проверка кода TypeScript | https://oxc.rs |
| mdBook | Сборка документации | https://rust-lang.github.io/mdBook |

## Данные

| Источник | Что взято |
|---|---|
| Каталог организаторов `catalog_export_v4` | Названия, производители, цены, отрасли, сценарии |
| Примеры решений организаторов (DOCX) | Технические характеристики части роботов |
| Демонстрационные параметры объектов организаторов (XLSX) | Основа демо-проектов и шаблона параметров |
| Страницы производителей и дистрибьюторов | Дополнительные технические характеристики. Адрес страницы записан в источнике каждого значения в файле `data/seeds/robot_specs.json` |

Правила внесения данных описаны в разделе [Данные и каталог](06-data.md).
