/**
 * ipatool GUI — Interactive Web Application Logic
 * Supports Russian (default) and English localization
 */

// Application State
const state = {
  lang: localStorage.getItem('ipatool_lang') || 'ru',
  theme: localStorage.getItem('ipatool_theme') || 'dark',
  activeTab: 'search',
  os: null, // 'windows' | 'darwin' | 'linux' | ..., from /api/status
  version: null,
  updateInfo: null,
  account: null,
  isAuthenticated: false,
  activeDownloads: new Map(), // jobId -> job object
  downloadHistory: JSON.parse(localStorage.getItem('ipatool_download_history') || '[]'),
  lastPendingLogin: null, // { email, password } for 2FA retry
  currentJobId: null,
  lastVersionsName: '', // app name remembered when opening version history from a search card
  versionsPage: null,   // current version-history page state: { ids, page, ctx }
  isDownloading: false, // prevent double-click on download buttons
  completedJobIds: new Set(), // prevent duplicate toasts/handlers for same job
  purchases: {
    apps: [],            // purchase history items from /api/purchases
    loaded: false,       // true once a successful response was received
    loading: false,
    error: '',
    fetchedAt: null,
    fromCache: false,
    loadedFor: null      // dsid the list belongs to
  }
};

// I18N Dictionaries
const i18n = {
  ru: {
    account_card_desc: 'Сохранённые аккаунты и активная сессия',
    account_card_title: 'Аккаунты Apple ID',
    account_remove_error: 'Ошибка удаления аккаунта',
    account_remove_failed: 'Не удалось удалить аккаунт',
    account_switch_error: 'Ошибка переключения аккаунта',
    account_switch_failed: 'Не удалось переключить Apple ID',
    accounts_active: 'Активный',
    accounts_empty: 'Добавьте Apple ID справа, чтобы переключаться между аккаунтами.',
    accounts_not_authenticated: 'Требуется вход',
    accounts_remove_confirm: 'Удалить этот Apple ID из программы? Сессия будет отозвана на этом компьютере.',
    accounts_removed_toast: 'Аккаунт удалён из программы',
    accounts_saved_title: 'Сохранённые аккаунты',
    accounts_switch: 'Переключить',
    accounts_switched_toast: 'Активный Apple ID изменён',
    acquire_license_tooltip: 'Получить лицензию',
    active_downloads_desc: 'Текущие процессы скачивания и обработки пакетов',
    active_downloads_title: 'Активные загрузки',
    anisette_alert_desc: 'Проверка установленного iCloud…',
    anisette_alert_title: 'Для Windows-версии ipatool:',
    anisette_card_desc: 'Проверьте, установлена ли нужная версия iCloud на этом компьютере',
    anisette_card_title: '⚙️ Проверка iCloud для Windows',
    app_subtitle: 'App Store IPA Downloader для Windows',
    auth_checking: 'Проверка аккаунта...',
    auth_logged_in: 'Авторизован',
    auth_not_logged_in: 'Не авторизован',
    auto_purchase_desc: 'Необходимо, если приложение ещё ни разу не скачивалось с этого Apple ID (только для бесплатных)',
    auto_purchase_title: 'Автоматически получить лицензию (Purchase)',
    badge_authenticated: 'Авторизован',
    batch_account_label: 'Apple ID для этого списка: {email}',
    batch_check_btn: 'Проверить по Apple ID',
    batch_check_item_label: 'Лимит на одну проверку (сек):',
    batch_check_progress_text: 'Проверено {done} из {total} (каждый прогон = {pct}% работы).',
    batch_check_progress_title: 'Проверка приложений через Прямую загрузку...',
    batch_check_running: 'Проверка уже выполняется',
    batch_check_start_error: 'Ошибка запуска проверки',
    batch_desc: 'Загрузите текстовый файл со списком приложений (название и App ID). Каждый ID будет проверен через Прямую загрузку: приложения, ранее установленные на вашем Apple ID, скачиваются, а те, которых у вас никогда не было (ошибка «license is required»), будут отфильтрованы.',
    batch_download_account_label: 'Apple ID загрузки: {email}',
    batch_download_done_title: 'Массовая загрузка завершена',
    batch_download_progress_done: 'Скачано {done} из {total}. Ошибок: {errors}.',
    batch_download_progress_title: 'Массовое скачивание выбранных приложений...',
    batch_download_selected: 'Скачать выбранные',
    batch_download_single: 'Скачать',
    batch_download_skipped_count: 'Пропущено: {skipped}.',
    batch_download_start_error: 'Ошибка запуска массовой загрузки',
    batch_download_started_toast: 'Загрузка началась — прогресс ниже',
    batch_download_status_completed: 'Готово',
    batch_download_status_downloading: 'Скачивание',
    batch_download_status_error: 'Ошибка',
    batch_download_status_patching: 'Подпись (sinf)',
    batch_download_status_purchasing: 'Лицензия',
    batch_download_status_queued: 'В очереди',
    batch_download_status_skipped: 'Пропущено',
    batch_download_status_stalled: 'Зависло (пропущено)',
    batch_file_hint: 'Каждая строка: название и числовой App ID, например 1Password 7: 568903335. Поддерживаются также просто ID, ссылки на App Store и разделители «:», «-», «;», Tab или пробел.',
    batch_file_label: 'Файл со списком приложений (.txt):',
    batch_filtered_error_title: 'Другие ошибки:',
    batch_filtered_title: 'Отфильтрованы (нет лицензии на Apple ID):',
    batch_item_hint: 'Жёсткий предел на все этапы одного приложения: лицензия + скачивание + подпись.',
    batch_item_label: 'Лимит на одно приложение (сек):',
    batch_latest_badge: 'Последняя',
    batch_need_auth: 'Сначала необходимо войти в Apple ID во вкладке «Аккаунт»',
    batch_no_items: 'Не удалось распознать App ID в списке',
    batch_no_selected: 'Выберите хотя бы одно приложение',
    batch_no_versions: 'Список версий недоступен',
    batch_parsed_count: 'Распознано приложений: {count}. Нажмите «Проверить по Apple ID», чтобы прогнать каждое через Прямую загрузку.',
    batch_paste_label: 'Или вставьте список текстом:',
    batch_paste_placeholder: `1Password 7: 568903335
2ГИС: 1474481751
568903335 246890568`,
    batch_purchase_hint: 'Включено: приложение без лицензии будет приобретено перед проверкой/скачиванием (только бесплатные приложения). Отключено: такие приложения будут просто отфильтрованы и не скачаны.',
    batch_purchase_label: 'Автоматически получать лицензии (Purchase)',
    batch_results_summary: 'Доступно для загрузки: {available} из {total}. Отфильтровано: {filtered}.',
    batch_results_title: 'Результаты проверки',
    batch_retry_failed: 'Повторить неудачные',
    batch_retry_running: 'Повтор уже выполняется',
    batch_save_list: 'Сохранить список (.txt)',
    batch_save_toast: 'Список сохранён в текстовый файл',
    batch_select_all: 'Выбрать все',
    batch_select_hint: 'Отметьте приложения и нажмите «Скачать выбранные»:',
    batch_select_none: 'Снять все',
    batch_skip_btn: 'Пропустить',
    batch_skip_error: 'Не удалось пропустить приложение',
    batch_skip_toast: 'Приложение пропущено — пакет продолжается',
    batch_stall_hint: 'Если приложение не отдаёт ни одного байта дольше этого времени, оно помечается «Зависло», и пакет переходит к следующему.',
    batch_stall_label: 'Считать зависшим, если нет данных (сек):',
    batch_stalled_hint: 'нет данных {sec} с',
    batch_stalled_status: 'нет ответа (пропущено)',
    batch_title: 'Массовая проверка и загрузка',
    batch_version_history: 'История версий',
    batch_version_history_close: 'Скрыть историю',
    batch_version_pick_hint: 'Отметьте версию, чтобы скачать именно её вместо последней (по кнопке «Скачать выбранные» вверху). Отметка снимает галочку с карточки.',
    batch_versions_loading: 'Получение данных о версиях...',
    cancel_btn: 'Отмена',
    close_btn: 'Закрыть',
    copied_toast: 'Скопировано в буфер обмена',
    copy_path_btn: 'Путь',
    copy_tooltip: 'Скопировать',
    default_user_name: 'Пользователь Apple',
    direct_desc: 'Загрузите приложение по Bundle ID, App ID или прямой ссылке на App Store',
    direct_id_hint: 'Вы можете указать Bundle Identifier (например com.example.app), цифровой Track ID (например 686449807) или ссылку на страницу в App Store.',
    direct_id_label: 'Идентификатор приложения (Bundle ID, App ID или Ссылка):',
    direct_id_placeholder: 'например: org.telegram.Telegram-iOS, 686449807 или https://apps.apple.com/...',
    direct_specify_id: 'Укажите Bundle ID или App ID',
    direct_title: 'Прямая загрузка приложения',
    dl_completed_ipa: 'Пакет .IPA успешно скачан!',
    dl_completed_pkg: 'Пакет .PKG успешно скачан!',
    dl_modal_default_subtitle: 'Подготовка к загрузке...',
    dl_modal_default_title: 'Скачивание приложения',
    dl_modal_download_failed: 'Произошла ошибка при скачивании',
    dl_modal_saved_toast: 'Файл сохранен: {path}',
    dl_modal_status_downloading: 'Скачивание зашифрованного пакета...',
    dl_modal_status_licensing: 'Получение лицензии App Store...',
    dl_modal_status_patching: 'Применение цифровой подписи (sinf)...',
    donate_button: 'Поддержать проект через Cloudtips',
    donate_desc: 'Если вы хотите безвозмездно поддержать проект, то можно это сделать по ссылке ниже или через QR-код:',
    donate_qr_caption: 'QR-код для поддержки проекта',
    donate_title: 'Поддержать проект',
    download_btn: 'Скачать IPA',
    download_btn_pkg: 'Скачать PKG',
    download_history_desc: 'Скачанные файлы .IPA на вашем компьютере',
    download_history_title: 'История завершенных загрузок',
    download_start_error: 'Ошибка запуска скачивания',
    enter_2fa_code: 'Введите 6-значный код',
    faq_a1: 'Серверы Apple App Store выдают установочные пакеты <code>.ipa</code> только авторизованным пользователям. Авторизация нужна, чтобы Apple сгенерировала персональную цифровую подпись (<code>sinf</code>) и лицензию на приложение для вашего аккаунта.',
    faq_a2: 'Да. Программа является полностью открытым исходным кодом (Open Source). Ваши учетные данные передаются исключительно на официальные защищенные серверы Apple (<code>auth.itunes.apple.com</code> / <code>gsa.apple.com</code>) с использованием криптографического протокола SRP-6a. Данные не передаются никаким сторонним серверам.',
    faq_a3: 'Это означает, что это приложение ещё никогда не приобреталось с вашего Apple ID. Просто нажмите кнопку <strong>"Получить лицензию"</strong> на карточке приложения или поставьте галочку <em>"Автоматически получить лицензию (Purchase)"</em> при скачивании.',
    faq_a4: 'Вы можете скачивать платные приложения, если вы ранее уже купили их со своего Apple ID. Бесплатное автоматическое приобретение лицензии через ipatool работает только для бесплатных приложений App Store.',
    faq_a5: 'Перейдите во вкладку <strong>"История версий"</strong> (или нажмите кнопку "Версии" на карточке приложения). Программа отобразит все когда-либо выпущенные версии приложения. Выберите нужную сборку и нажмите "Скачать".',
    faq_a6: 'По умолчанию файлы сохраняются в папку <code>apps</code> внутри рабочей папки программы. Вы можете открыть её в любой момент, нажав кнопку <strong>"📂 Открыть папку загрузок"</strong> во вкладке "Загрузки".',
    faq_a7: '<p>Если вы получаете ошибку <code>invalid response: Apple Media Services Terms and Conditions have changed</code>, это означает, что Apple обновила условия использования, и ваш аккаунт ещё не принял новые условия.</p><p><strong data-i18n="faq_a7_fix_title">Как исправить:</strong></p><ol><li data-i18n="faq_a7_step1">Откройте App Store на вашем iPhone, iPad или Mac</li><li data-i18n="faq_a7_step2">Войдите в тот же Apple ID, если потребуется</li><li data-i18n="faq_a7_step3">Начните скачивание любого приложения (можно попробовать то, с которым была ошибка, например Insta360) - это должно вызвать окно с новыми условиями</li><li data-i18n="faq_a7_step4">Примите новые условия и соглашения при появлении запроса</li><li data-i18n="faq_a7_step5">После принятия условий ipatool должен снова работать нормально</li></ol><p data-i18n="faq_a7_note">Это стандартная процедура Apple при обновлении их юридических условий. После принятия через официальное приложение Apple ошибка будет устранена.</p>',
    faq_a7_fix_title: 'Как исправить:',
    faq_a7_note: 'Это стандартная процедура Apple при обновлении их юридических условий. После принятия через официальное приложение Apple ошибка будет устранена.',
    faq_a7_step1: 'Откройте App Store на вашем iPhone, iPad или Mac',
    faq_a7_step2: 'Войдите в тот же Apple ID, если потребуется',
    faq_a7_step3: 'Начните скачивание любого приложения (можно попробовать то, с которым была ошибка, например Insta360) - это должно вызвать окно с новыми условиями',
    faq_a7_step4: 'Примите новые условия и соглашения при появлении запроса',
    faq_a7_step5: 'После принятия условий ipatool должен снова работать нормально',
    faq_q1: 'Зачем нужен вход в Apple ID?',
    faq_q2: 'Безопасно ли вводить пароль от Apple ID в ipatool?',
    faq_q3: 'Что делать, если при скачивании пишет "license required"?',
    faq_q4: 'Можно ли скачивать платные приложения?',
    faq_q5: 'Как скачать старую версию приложения?',
    faq_q6: 'Где сохраняются скачанные файлы .IPA?',
    faq_q7: 'Что делать, если при скачивании появляется ошибка "invalid response"?',
    faq_title: 'Часто задаваемые вопросы (FAQ)',
    fetch_versions_btn: 'Получить версии',
    fetching_versions_status: 'Запрос списка сборок с серверов Apple...',
    filename_warning: 'Не удалось прочитать имя и версию из Info.plist; имя файла не изменено.',
    free_price: 'Бесплатно',
    guide_desc: 'Скачанный файл .IPA является официальным пакетом App Store. Вот лучшие способы установить его на ваше устройство:',
    guide_title: '📱 Инструкция: Как установить .IPA на iPhone или iPad',
    history_refreshed: 'Список обновлен',
    icloud_download_btn: 'Скачать классическую iCloud для Windows',
    icloud_installed: '✅ Классическая iCloud для Windows установлена.',
    icloud_installed_classic: 'Найдена папка Apple\\\\Internet Services с AOSKit.dll.',
    icloud_not_installed: '❌ Классическая iCloud для Windows не найдена. Скачайте и установите её по ссылке ниже, затем войдите в неё своим Apple ID.',
    icloud_store_url: 'https://updates.cdn-apple.com/2020/windows/001-39935-20200911-1A70AA56-F448-11EA-8CC0-99D41950005E/iCloudSetup.exe',
    in_explorer_btn: 'В проводнике',
    info_country: 'Регион App Store:',
    info_dsid: 'DSID:',
    info_keychain: 'Хранилище сессии:',
    info_keychain_ok: 'Изолированное хранилище профиля',
    install_button: 'Установить на устройство',
    install_completed: 'Приложение успешно установлено!',
    install_device_label: 'Устройство:',
    install_device_select_placeholder: '— выберите устройство —',
    install_devices_desc: 'Обнаруживаются автоматически через libimobiledevice (idevice_id / idevicedeviceinfo)',
    install_devices_refresh_error: 'Не удалось получить список устройств',
    install_devices_scanning: 'Поиск подключенных устройств...',
    install_devices_title: 'Подключенные устройства iOS',
    install_driver_check_title: 'Драйвер Apple Mobile Device Support',
    install_driver_checking: 'Проверка драйвера...',
    install_driver_download: 'Скачать Apple Mobile Device Support',
    install_driver_itunes: 'Скачать iTunes',
    install_driver_missing: 'Драйвер (служба) Apple Mobile Device Support не найден. Установите iTunes, чтобы получить его, или обновите драйвер USB для устройства.',
    install_driver_ok: 'Драйвер Apple Mobile Device Support найден. USB-подключение готово к работе.',
    install_driver_unknown: 'Проверка драйвера недоступна на этой системе.',
    install_dropzone_text: 'Нажмите, чтобы выбрать .ipa файл',
    install_error: 'Ошибка установки',
    install_file_label: 'Файл .IPA:',
    install_file_selected: 'Выбран файл:',
    install_log_title: 'Журнал ideviceinstaller',
    install_needs_device: 'Выберите подключенное устройство',
    install_needs_file: 'Выберите файл .IPA',
    install_no_devices: 'Подключите iPhone/iPad по кабелю и нажмите «Обновить»',
    install_progress_title: 'Установка...',
    install_started_toast: 'Установка началась — прогресс ниже',
    install_status_completed: 'Готово',
    install_status_error: 'Ошибка',
    install_status_installing: 'Установка',
    install_status_queued: 'В очереди',
    install_tools_missing_desc: 'Для установки нужен ideviceinstaller из libimobiledevice. На macOS: brew install libimobiledevice. На Windows установите iDevice Suite / сборку libimobiledevice и положите ideviceinstaller.exe в PATH.',
    install_tools_missing_title: 'Не найден ideviceinstaller',
    install_upload_desc: 'Выберите скачанный .IPA, укажите устройство и установите его на iPhone/iPad',
    install_upload_title: 'Выберите файл .IPA',
    license_already_owned: 'Лицензия уже имеется на вашем Apple ID',
    license_btn: 'Лицензия',
    license_error: 'Ошибка приобретения лицензии',
    license_success: 'Лицензия успешно получена!',
    limit_label: 'Результатов:',
    login_2fa_required: 'Требуется код двухфакторной аутентификации',
    login_anisette_error: 'Ошибка iCloud (anisette): {msg}',
    login_auth_error: 'Ошибка авторизации Apple ID',
    login_btn: 'Добавить Apple ID',
    login_email_label: 'Apple ID (Email):',
    login_fill_fields: 'Заполните Email и Пароль',
    login_password_label: 'Пароль Apple ID:',
    login_prompt_desc: 'Добавьте ещё один Apple ID. Аккаунты сохраняются отдельно и переключаются без повторного входа.',
    login_prompt_title: 'Добавить Apple ID',
    login_security_hint: 'Пароль передаётся Apple по защищённому соединению и хранится только в защищённом хранилище профиля для обновления сессии; в accounts.json он не записывается. При запросе 2FA пароль временно хранится в памяти сервера не более 10 минут.',
    login_submitting: 'Авторизация...',
    login_success_toast: 'Успешный вход: {email}',
    logout_btn: 'Удалить аккаунт из программы',
    min_ios_badge_title: 'Минимальная поддерживаемая версия iOS',
    modal_2fa_desc: 'Введите 6-значный проверочный код, отправленный на ваши устройства Apple или в SMS',
    modal_2fa_title: 'Двухфакторная аутентификация',
    need_account_for_download: 'Для скачивания необходимо войти в Apple ID',
    need_account_for_license: 'Сначала необходимо войти в Apple ID во вкладке "Аккаунт"',
    no_active_downloads: 'Нет активных загрузок',
    no_history_downloads: 'История пока пуста',
    no_results_desc: 'Попробуйте изменить поисковый запрос или переключить платформу (iPhone / iPad).',
    no_results_title: 'Ничего не найдено',
    no_sinf_short: 'без sinf',
    no_sinf_warning: 'Apple отдала пакет без DRM-подписей (sinf). Файл .ipa сохранён полностью — его можно расшифровать или изучить, но на устройство он не установится. Попробуйте скачать позже или выберите другую версию.',
    open_downloads_folder: 'Открыть папку загрузок',
    open_folder_btn: 'Открыть',
    open_folder_error: 'Ошибка при открытии папки',
    open_folder_failed: 'Не удалось открыть папку',
    open_in_explorer_btn: 'Показать в проводнике',
    open_output_folder_title: 'Открыть папku сохранения',
    output_folder_label: 'Папка для сохранения:',
    output_folder_placeholder: 'По умолчанию: папка apps в текущей папке',
    pager_all: 'Все',
    pager_first: '« Первая',
    pager_last: 'Последняя »',
    pager_next: 'Вперёд ›',
    pager_page_of: 'Стр. {page} из {pages}',
    pager_per_page: 'На странице:',
    pager_prev: '‹ Назад',
    pager_range: '{from}–{to} из {total}',
    platform_label: 'Платформа:',
    popular_searches: 'Популярное:',
    purchases_cache_stale_title: 'Кэш истории покупок не обновлялся более 7 дней',
    purchases_col_action: 'Действие',
    purchases_col_app: 'Приложение',
    purchases_col_appid: 'App ID',
    purchases_col_bundle: 'Bundle ID',
    purchases_col_date: 'Дата покупки',
    purchases_desc: 'Все приложения, приобретённые на этом Apple ID. Список загружается один раз при входе и обновляется по кнопке.',
    purchases_download: 'Скачать',
    purchases_empty: 'На этом Apple ID нет приобретённых приложений',
    purchases_export: 'Сохранить список (.txt)',
    purchases_filter_placeholder: 'Фильтр по названию, Bundle ID или App ID',
    purchases_from_cache: 'из кэша',
    purchases_load_failed: 'Не удалось загрузить историю покупок',
    purchases_loaded_toast: 'История покупок: {count} прил.',
    purchases_loading: 'Загрузка истории покупок с серверов Apple...',
    purchases_need_auth: 'Войдите в Apple ID во вкладке «Аккаунт», чтобы увидеть историю покупок',
    purchases_no_match: 'Ничего не найдено по фильтру',
    purchases_nothing_to_export: 'Список пуст — нечего сохранять',
    purchases_only_hidden: 'Только скрытые',
    purchases_only_hidden_title: 'Показать только приложения, скрытые или удалённые из региональной витрины App Store',
    purchases_os_all: 'Все ОС',
    purchases_os_label: 'ОС:',
    purchases_refresh: 'Обновить',
    purchases_retry: 'Повторить',
    purchases_save_failed: 'Не удалось сохранить список в файл',
    purchases_saved_toast: 'Список сохранён: {path}',
    purchases_title: 'История покупок',
    purchases_unknown_app: 'Без названия',
    purchases_updated_at: 'Обновлено: {time}',
    purchases_versions: 'Версии',
    refresh_btn: 'Обновить',
    remove_from_list_title: 'Удалить из списка',
    removed_badge: 'Удалено из App Store',
    removed_found: 'Найдено удалённых: {count}',
    requesting_license: 'Запрос лицензии для {bundleId}...',
    results_found: 'Найдено приложений: {count}',
    search_button: 'Найти',
    search_desc: 'Введите название приложения или вставьте ссылку на App Store',
    search_error: 'Ошибка поиска в App Store',
    search_official_title: 'Результаты App Store',
    search_placeholder: 'Например: Telegram, Minecraft, VK или https://apps.apple.com/app/id...',
    search_prompt_desc: 'Найдите нужное приложение, получите для него лицензию и скачайте зашифрованный установочный пакет .IPA',
    search_prompt_title: 'Введите название для поиска',
    search_query_empty: 'Введите поисковый запрос',
    search_removed_title: 'Удалённые из App Store (доступны по ID)',
    search_title: 'Поиск приложений в App Store',
    searching_status: 'Поиск приложений в Apple App Store...',
    server_connection_error: 'Ошибка связи с сервером',
    start_download_btn: 'Начать скачивание .IPA',
    status_error_label: 'Ошибка',
    step_complete: 'Готово к установке',
    step_download: 'Загрузка пакета .IPA из App Store',
    step_download_pkg: 'Загрузка пакета .PKG из App Store',
    step_license: 'Проверка лицензии / Покупка',
    step_sinf: 'Применение цифровой подписи (sinf)',
    tab_account: 'Аккаунт Apple ID',
    tab_batch: 'Массовая загрузка',
    tab_direct: 'Прямая загрузка',
    tab_donate: 'Донат',
    tab_downloads: 'Загрузки',
    tab_guide: 'Инструкция и FAQ',
    tab_install: 'Установка на устройство',
    tab_purchases: 'История покупок',
    tab_search: 'Поиск приложений',
    tab_versions: 'История версий',
    theme_toggle_title: 'Сменить тему',
    update_available_tip: 'Доступна новая версия: {latest} (текущая: {current}) — нажмите, чтобы открыть страницу релизов',
    update_pill_new: '⬆️ Новая: {latest}',
    verify_btn: 'Подтвердить',
    verifying_code: 'Проверка...',
    version_col_action: 'Действие',
    version_col_build: 'ID сборки (Version ID)',
    version_col_date: 'Дата выхода',
    version_col_display: 'Версия (Display)',
    version_col_select: 'Выбрать',
    version_current_tip: 'Текущая версия: {current}',
    version_history_tooltip: 'История версий',
    version_id_label: 'ID версии (необязательно):',
    version_id_placeholder: 'Оставьте пустым для последней версии',
    versions_app_default: 'Приложение',
    versions_btn: 'Версии',
    versions_count_badge: '{count} версий',
    versions_desc: 'Просмотр всех предыдущих сборок и скачивание старых версий приложения',
    versions_fetch_failed: 'Не удалось получить версии приложения',
    versions_search_placeholder: 'Введите Bundle ID (например, org.telegram.Telegram-iOS) или App ID',
    versions_title: 'История версий приложения',
    window_title: 'ipatool webGUI — Скачивание IPA из App Store'
  },
  en: {
    account_card_desc: 'Saved accounts and the active session',
    account_card_title: 'Apple ID Accounts',
    account_remove_error: 'Failed to remove account',
    account_remove_failed: 'Could not remove account',
    account_switch_error: 'Failed to switch account',
    account_switch_failed: 'Could not switch Apple ID',
    accounts_active: 'Active',
    accounts_empty: 'Add an Apple ID on the right to switch between accounts.',
    accounts_not_authenticated: 'Sign-in required',
    accounts_remove_confirm: 'Remove this Apple ID from the app? Its session will be revoked on this computer.',
    accounts_removed_toast: 'Account removed from the app',
    accounts_saved_title: 'Saved accounts',
    accounts_switch: 'Switch',
    accounts_switched_toast: 'Active Apple ID changed',
    acquire_license_tooltip: 'Acquire license',
    active_downloads_desc: 'Current download and package signing processes',
    active_downloads_title: 'Active Downloads',
    anisette_alert_desc: 'Checking installed iCloud…',
    anisette_alert_title: 'For Windows users of ipatool:',
    anisette_card_desc: 'Verify that the required iCloud version is installed on this computer',
    anisette_card_title: '⚙️ iCloud Check for Windows',
    app_subtitle: 'App Store IPA Downloader for Windows',
    auth_checking: 'Checking account...',
    auth_logged_in: 'Authenticated',
    auth_not_logged_in: 'Not Logged In',
    auto_purchase_desc: 'Required if this app was never downloaded with this Apple ID before (free apps only)',
    auto_purchase_title: 'Auto-acquire license (Purchase)',
    badge_authenticated: 'Authenticated',
    batch_account_label: 'Apple ID for this list: {email}',
    batch_check_btn: 'Check with Apple ID',
    batch_check_item_label: 'Per-check time limit (sec):',
    batch_check_progress_text: 'Checked {done} of {total} (each pass = {pct}% of the work).',
    batch_check_progress_title: 'Checking apps through Direct Download...',
    batch_check_running: 'A check is already running',
    batch_check_start_error: 'Failed to start batch check',
    batch_desc: 'Upload a text file with a list of apps (name and App ID). Each ID is run through the Direct Download path: apps previously installed on your Apple ID work, while apps you never installed (the "license is required" error) are filtered out.',
    batch_download_account_label: 'Download account: {email}',
    batch_download_done_title: 'Mass download finished',
    batch_download_progress_done: 'Downloaded {done} of {total}. Errors: {errors}.',
    batch_download_progress_title: 'Mass downloading selected apps...',
    batch_download_selected: 'Download selected',
    batch_download_single: 'Download',
    batch_download_skipped_count: 'Skipped: {skipped}.',
    batch_download_start_error: 'Failed to start mass download',
    batch_download_started_toast: 'Download started — progress is below',
    batch_download_status_completed: 'Done',
    batch_download_status_downloading: 'Downloading',
    batch_download_status_error: 'Error',
    batch_download_status_patching: 'Signing (sinf)',
    batch_download_status_purchasing: 'License',
    batch_download_status_queued: 'Queued',
    batch_download_status_skipped: 'Skipped',
    batch_download_status_stalled: 'Stalled (skipped)',
    batch_file_hint: 'One entry per line: a name and a numeric App ID, e.g. 1Password 7: 568903335. Plain IDs, App Store URLs and separators ":", "-", ";", Tab or space are also supported.',
    batch_file_label: 'App list file (.txt):',
    batch_filtered_error_title: 'Other errors:',
    batch_filtered_title: 'Filtered out (no license on Apple ID):',
    batch_item_hint: 'Hard cap for all phases of one app: license + download + signing.',
    batch_item_label: 'Per-app time limit (sec):',
    batch_latest_badge: 'Latest',
    batch_need_auth: 'Sign in to your Apple ID in the "Account" tab first',
    batch_no_items: 'Could not recognize an App ID in the list',
    batch_no_selected: 'Select at least one app',
    batch_no_versions: 'Version list unavailable',
    batch_parsed_count: 'Recognized apps: {count}. Press "Check with Apple ID" to run each one through Direct Download.',
    batch_paste_label: 'Or paste the list as text:',
    batch_paste_placeholder: `1Password 7: 568903335
Telegram: 686449807
568903335 246890568`,
    batch_purchase_hint: 'When enabled, an app without a license is purchased before checking/downloading (free apps only). When disabled, those apps are filtered out and not downloaded.',
    batch_purchase_label: 'Automatically acquire licenses (Purchase)',
    batch_results_summary: 'Available for download: {available} of {total}. Filtered out: {filtered}.',
    batch_results_title: 'Check Results',
    batch_retry_failed: 'Retry failed',
    batch_retry_running: 'A retry is already running',
    batch_save_list: 'Save list (.txt)',
    batch_save_toast: 'List saved to a text file',
    batch_select_all: 'Select all',
    batch_select_hint: 'Check the apps and press "Download selected":',
    batch_select_none: 'Select none',
    batch_skip_btn: 'Skip',
    batch_skip_error: 'Could not skip the app',
    batch_skip_toast: 'App skipped — the batch moves on',
    batch_stall_hint: 'If an app does not deliver a single byte for longer than this, it is marked "Stalled" and the batch moves on to the next one.',
    batch_stall_label: 'Treat as stalled when no data arrives (sec):',
    batch_stalled_hint: 'no data for {sec}s',
    batch_stalled_status: 'no response (skipped)',
    batch_title: 'Mass Check & Download',
    batch_version_history: 'Version history',
    batch_version_history_close: 'Hide history',
    batch_version_pick_hint: 'Tick a version to download it instead of the latest (via the "Download selected" button above). Ticking it clears the card checkbox.',
    batch_versions_loading: 'Fetching version data...',
    cancel_btn: 'Cancel',
    close_btn: 'Close',
    copied_toast: 'Copied to clipboard',
    copy_path_btn: 'Copy Path',
    copy_tooltip: 'Copy',
    default_user_name: 'Apple User',
    direct_desc: 'Download an app by Bundle ID, App ID or direct App Store URL',
    direct_id_hint: 'You can provide a Bundle Identifier (e.g. com.example.app), numeric Track ID (e.g. 686449807), or an App Store web link.',
    direct_id_label: 'App Identifier (Bundle ID, App ID or URL):',
    direct_id_placeholder: 'e.g. org.telegram.Telegram-iOS, 686449807 or https://apps.apple.com/...',
    direct_specify_id: 'Please specify a Bundle ID or App ID',
    direct_title: 'Direct App Download',
    dl_completed_ipa: 'Package .IPA downloaded successfully!',
    dl_completed_pkg: 'Package .PKG downloaded successfully!',
    dl_modal_default_subtitle: 'Preparing download...',
    dl_modal_default_title: 'Downloading App',
    dl_modal_download_failed: 'An error occurred during download',
    dl_modal_saved_toast: 'File saved: {path}',
    dl_modal_status_downloading: 'Downloading encrypted package...',
    dl_modal_status_licensing: 'Acquiring App Store license...',
    dl_modal_status_patching: 'Applying digital signature (sinf)...',
    donate_button: 'Support the project via Cloudtips',
    donate_desc: 'If you would like to support the project free of charge, you can do so via the link below or by scanning the QR code:',
    donate_qr_caption: 'QR code to support the project',
    donate_title: 'Support the project',
    download_btn: 'Download IPA',
    download_btn_pkg: 'Download PKG',
    download_history_desc: 'Completed .IPA packages on your computer',
    download_history_title: 'Download History',
    download_start_error: 'Failed to start download',
    enter_2fa_code: 'Please enter the 6-digit code',
    faq_a1: 'Apple App Store servers only issue <code>.ipa</code> installation packages to authorized users. Authorization is required so Apple can generate a personalized digital signature (<code>sinf</code>) and license for your account.',
    faq_a2: 'Yes. The program is fully open source. Your credentials are sent exclusively to official Apple secure servers (<code>auth.itunes.apple.com</code> / <code>gsa.apple.com</code>) using the SRP-6a cryptographic protocol. Data is not sent to any third-party servers.',
    faq_a3: 'This means this app has never been purchased with your Apple ID. Simply click the <strong>"Get License"</strong> button on the app card or check <em>"Auto-get license (Purchase)"</em> when downloading.',
    faq_a4: 'You can download paid apps if you have previously purchased them with your Apple ID. Free automatic license acquisition through ipatool only works for free App Store apps.',
    faq_a5: 'Go to the <strong>"Version History"</strong> tab (or click the "Versions" button on the app card). The program will display all ever released versions of the app. Select the desired build and click "Download".',
    faq_a6: 'By default, files are saved to the <code>apps</code> folder inside the program\\\'s working directory. You can open the folder at any time by clicking the <strong>"📂 Open downloads folder"</strong> button in the "Downloads" tab.',
    faq_a7: '<p>If you get the error <code>invalid response: Apple Media Services Terms and Conditions have changed</code>, it means Apple has updated their terms of use and your account has not accepted the new terms yet.</p><p><strong data-i18n="faq_a7_fix_title">How to fix:</strong></p><ol><li data-i18n="faq_a7_step1">Open the App Store on your iPhone, iPad, or Mac</li><li data-i18n="faq_a7_step2">Sign in with the same Apple ID if prompted</li><li data-i18n="faq_a7_step3">Start downloading any app (you can try the one that gave you the error, e.g., Insta360) - this should trigger the new terms popup</li><li data-i18n="faq_a7_step4">Accept the new terms and conditions when prompted</li><li data-i18n="faq_a7_step5">After accepting the terms, ipatool should work normally again</li></ol><p data-i18n="faq_a7_note">This is a standard Apple procedure when they update their legal terms. Once you accept them through an official Apple app, the error will be resolved.</p>',
    faq_a7_fix_title: 'How to fix:',
    faq_a7_note: 'This is a standard Apple procedure when they update their legal terms. Once you accept them through an official Apple app, the error will be resolved.',
    faq_a7_step1: 'Open the App Store on your iPhone, iPad, or Mac',
    faq_a7_step2: 'Sign in with the same Apple ID if prompted',
    faq_a7_step3: 'Start downloading any app (you can try the one that gave you the error, e.g., Insta360) - this should trigger the new terms popup',
    faq_a7_step4: 'Accept the new terms and conditions when prompted',
    faq_a7_step5: 'After accepting the terms, ipatool should work normally again',
    faq_q1: 'Why do I need to sign in with Apple ID?',
    faq_q2: 'Is it safe to enter my Apple ID password in ipatool?',
    faq_q3: 'What to do if download says "license required"?',
    faq_q4: 'Can I download paid apps?',
    faq_q5: 'How to download an old version of an app?',
    faq_q6: 'Where are downloaded .IPA files saved?',
    faq_q7: 'What to do if "invalid response" error occurs during download?',
    faq_title: 'Frequently Asked Questions (FAQ)',
    fetch_versions_btn: 'Fetch Versions',
    fetching_versions_status: 'Fetching build list from Apple servers...',
    filename_warning: 'Could not read the app name and version from Info.plist; the downloaded filename was left unchanged.',
    free_price: 'Free',
    guide_desc: 'Downloaded .IPA files are genuine App Store packages. Here are the best methods to install them:',
    guide_title: '📱 Guide: How to install .IPA on iPhone or iPad',
    history_refreshed: 'List refreshed',
    icloud_download_btn: 'Download classic iCloud for Windows',
    icloud_installed: '✅ Classic iCloud for Windows is installed.',
    icloud_installed_classic: 'Found Apple\\\\Internet Services with AOSKit.dll.',
    icloud_not_installed: '❌ Classic iCloud for Windows was not found. Download and install it via the link below, then sign in with your Apple ID.',
    icloud_store_url: 'https://updates.cdn-apple.com/2020/windows/001-39935-20200911-1A70AA56-F448-11EA-8CC0-99D41950005E/iCloudSetup.exe',
    in_explorer_btn: 'Show in Explorer',
    info_country: 'App Store Storefront:',
    info_dsid: 'DSID:',
    info_keychain: 'Session Storage:',
    info_keychain_ok: 'Isolated profile keychain',
    install_button: 'Install on device',
    install_completed: 'App installed successfully!',
    install_device_label: 'Device:',
    install_device_select_placeholder: '— select a device —',
    install_devices_desc: 'Detected automatically with libimobiledevice (idevice_id / idevicedeviceinfo)',
    install_devices_refresh_error: 'Failed to get device list',
    install_devices_scanning: 'Scanning for connected devices...',
    install_devices_title: 'Connected iOS devices',
    install_driver_check_title: 'Apple Mobile Device Support driver',
    install_driver_checking: 'Checking driver...',
    install_driver_download: 'Download Apple Mobile Device Support',
    install_driver_itunes: 'Download iTunes',
    install_driver_missing: 'Apple Mobile Device Support driver/service not found. Install iTunes (which bundles it) or update the Apple USB driver for the device.',
    install_driver_ok: 'Apple Mobile Device Support driver found. USB connection is ready.',
    install_driver_unknown: 'Driver check is not available on this system.',
    install_dropzone_text: 'Click to select an .ipa file',
    install_error: 'Installation failed',
    install_file_label: 'IPA file:',
    install_file_selected: 'Selected file:',
    install_log_title: 'ideviceinstaller log',
    install_needs_device: 'Select a connected device',
    install_needs_file: 'Select an .ipa file',
    install_no_devices: 'Connect an iPhone/iPad by cable and press "Refresh"',
    install_progress_title: 'Installing...',
    install_started_toast: 'Installation started — progress is below',
    install_status_completed: 'Done',
    install_status_error: 'Error',
    install_status_installing: 'Installing',
    install_status_queued: 'Queued',
    install_tools_missing_desc: 'Installing requires ideviceinstaller from libimobiledevice. On macOS: brew install libimobiledevice. On Windows install iDevice Suite / a libimobiledevice build and put ideviceinstaller.exe on PATH.',
    install_tools_missing_title: 'ideviceinstaller not found',
    install_upload_desc: 'Select a downloaded .IPA, choose the device and install it onto iPhone/iPad',
    install_upload_title: 'Choose an .IPA file',
    license_already_owned: 'License is already owned by your Apple ID',
    license_btn: 'License',
    license_error: 'Failed to acquire license',
    license_success: 'License acquired successfully!',
    limit_label: 'Results:',
    login_2fa_required: 'Two-factor authentication code required',
    login_anisette_error: 'iCloud (anisette) error: {msg}',
    login_auth_error: 'Apple ID authentication error',
    login_btn: 'Add Apple ID',
    login_email_label: 'Apple ID (Email):',
    login_fill_fields: 'Please enter Email and Password',
    login_password_label: 'Apple ID Password:',
    login_prompt_desc: 'Add another Apple ID. Accounts are stored separately and can be switched without signing in again.',
    login_prompt_title: 'Add an Apple ID',
    login_security_hint: 'Your password is sent to Apple over a secure connection and stored only in the profile keychain so ipatool can refresh the session; it is not written to accounts.json. During a 2FA challenge, the password is held temporarily in server memory for up to 10 minutes.',
    login_submitting: 'Signing in...',
    login_success_toast: 'Successfully signed in: {email}',
    logout_btn: 'Remove account from app',
    min_ios_badge_title: 'Minimum supported iOS version',
    modal_2fa_desc: 'Enter the 6-digit verification code sent to your Apple devices or SMS',
    modal_2fa_title: 'Two-Factor Authentication',
    need_account_for_download: 'Sign in to Apple ID is required to download',
    need_account_for_license: 'Sign in to Apple ID in the "Account" tab first',
    no_active_downloads: 'No active downloads',
    no_history_downloads: 'History is empty',
    no_results_desc: 'Try refining your search keyword or change platform (iPhone / iPad).',
    no_results_title: 'No Results Found',
    no_sinf_short: 'no sinf',
    no_sinf_warning: 'Apple served the package without DRM signatures (sinf). The .ipa was saved completely — it can be decrypted or inspected, but it will not install on a device. Try again later or pick a different version.',
    open_downloads_folder: 'Open Downloads Folder',
    open_folder_btn: 'Open',
    open_folder_error: 'Error opening folder',
    open_folder_failed: 'Failed to open folder',
    open_in_explorer_btn: 'Show in Explorer',
    open_output_folder_title: 'Open output folder',
    output_folder_label: 'Output Folder:',
    output_folder_placeholder: 'Default: apps folder in current directory',
    pager_all: 'All',
    pager_first: '« First',
    pager_last: 'Last »',
    pager_next: 'Next ›',
    pager_page_of: 'Page {page} of {pages}',
    pager_per_page: 'Per page:',
    pager_prev: '‹ Prev',
    pager_range: '{from}–{to} of {total}',
    platform_label: 'Platform:',
    popular_searches: 'Popular:',
    purchases_cache_stale_title: 'Purchase history cache has not been updated for more than 7 days',
    purchases_col_action: 'Action',
    purchases_col_app: 'App',
    purchases_col_appid: 'App ID',
    purchases_col_bundle: 'Bundle ID',
    purchases_col_date: 'Purchased',
    purchases_desc: 'Every app owned by this Apple ID. The list is fetched once after sign-in and refreshed on demand.',
    purchases_download: 'Download',
    purchases_empty: 'This Apple ID has not acquired any apps',
    purchases_export: 'Save list (.txt)',
    purchases_filter_placeholder: 'Filter by name, Bundle ID or App ID',
    purchases_from_cache: 'cached',
    purchases_load_failed: 'Failed to load purchase history',
    purchases_loaded_toast: 'Purchase history: {count} apps',
    purchases_loading: 'Loading purchase history from Apple...',
    purchases_need_auth: 'Sign in to your Apple ID on the "Account" tab to see the purchase history',
    purchases_no_match: 'Nothing matches the filter',
    purchases_nothing_to_export: 'The list is empty — nothing to save',
    purchases_only_hidden: 'Hidden only',
    purchases_only_hidden_title: 'Show only apps hidden or removed from the regional App Store storefront',
    purchases_os_all: 'All OS',
    purchases_os_label: 'OS:',
    purchases_refresh: 'Refresh',
    purchases_retry: 'Retry',
    purchases_save_failed: 'Failed to save list to file',
    purchases_saved_toast: 'List saved: {path}',
    purchases_title: 'Purchase History',
    purchases_unknown_app: 'Untitled',
    purchases_updated_at: 'Updated: {time}',
    purchases_versions: 'Versions',
    refresh_btn: 'Refresh',
    remove_from_list_title: 'Remove from list',
    removed_badge: 'Removed from App Store',
    removed_found: 'Found removed apps: {count}',
    requesting_license: 'Requesting license for {bundleId}...',
    results_found: 'Found apps: {count}',
    search_button: 'Search',
    search_desc: 'Enter an app title or paste an App Store URL',
    search_error: 'App Store search error',
    search_official_title: 'App Store Results',
    search_placeholder: 'e.g. Telegram, Minecraft, VK or https://apps.apple.com/app/id...',
    search_prompt_desc: 'Find any iOS/iPadOS/tvOS app, acquire free licenses and download encrypted .IPA packages',
    search_prompt_title: 'Search for Apps',
    search_query_empty: 'Please enter a search query',
    search_removed_title: 'Removed from the App Store (available by ID)',
    search_title: 'Search Apple App Store',
    searching_status: 'Searching apps in Apple App Store...',
    server_connection_error: 'Server connection error',
    start_download_btn: 'Start .IPA Download',
    status_error_label: 'Error',
    step_complete: 'Ready for installation',
    step_download: 'Downloading .IPA package from App Store',
    step_download_pkg: 'Downloading .PKG package from App Store',
    step_license: 'License Verification / Purchase',
    step_sinf: 'Applying digital signature (sinf replication)',
    tab_account: 'Apple ID Account',
    tab_batch: 'Mass Download',
    tab_direct: 'Direct Download',
    tab_donate: 'Donate',
    tab_downloads: 'Downloads',
    tab_guide: 'Guide & FAQ',
    tab_install: 'Install to device',
    tab_purchases: 'Purchase History',
    tab_search: 'Search Apps',
    tab_versions: 'Version History',
    theme_toggle_title: 'Toggle theme',
    update_available_tip: 'New version available: {latest} (current: {current}) — click to open Releases',
    update_pill_new: '⬆️ New: {latest}',
    verify_btn: 'Verify Code',
    verifying_code: 'Verifying...',
    version_col_action: 'Action',
    version_col_build: 'Build ID (Version ID)',
    version_col_date: 'Release Date',
    version_col_display: 'Display Version',
    version_col_select: 'Select',
    version_current_tip: 'Current version: {current}',
    version_history_tooltip: 'Version history',
    version_id_label: 'Version ID (optional):',
    version_id_placeholder: 'Leave empty for latest version',
    versions_app_default: 'App',
    versions_btn: 'Versions',
    versions_count_badge: '{count} versions',
    versions_desc: 'View all historical builds and download previous app versions',
    versions_fetch_failed: 'Failed to fetch app versions',
    versions_search_placeholder: 'Enter Bundle ID (e.g. org.telegram.Telegram-iOS) or App ID',
    versions_title: 'App Version History',
    window_title: 'ipatool webGUI — App Store IPA Downloader'
  },
  vi: {
    account_card_desc: 'Các tài khoản đã lưu và phiên đăng nhập hiện tại',
    account_card_title: 'Tài khoản Apple ID',
    account_remove_error: 'Lỗi xóa tài khoản',
    account_remove_failed: 'Không thể xóa tài khoản',
    account_switch_error: 'Lỗi chuyển đổi tài khoản',
    account_switch_failed: 'Không thể chuyển đổi Apple ID',
    accounts_active: 'Đang dùng',
    accounts_empty: 'Thêm tài khoản Apple ID ở khung bên phải để chuyển đổi giữa các tài khoản.',
    accounts_not_authenticated: 'Cần đăng nhập',
    accounts_remove_confirm: 'Xóa Apple ID này khỏi ứng dụng? Phiên đăng nhập trên máy tính này sẽ bị thu hồi.',
    accounts_removed_toast: 'Đã xóa tài khoản khỏi ứng dụng',
    accounts_saved_title: 'Tài khoản đã lưu',
    accounts_switch: 'Chuyển',
    accounts_switched_toast: 'Đã chuyển đổi tài khoản Apple ID',
    acquire_license_tooltip: 'Nhận bản quyền',
    active_downloads_desc: 'Quá trình tải xuống và áp chữ ký số (sinf) hiện tại',
    active_downloads_title: 'Tải xuống đang diễn ra',
    anisette_alert_desc: 'Đang kiểm tra cài đặt iCloud…',
    anisette_alert_title: 'Dành cho người dùng ipatool trên Windows:',
    anisette_card_desc: 'Xác minh phiên bản iCloud yêu cầu đã được cài đặt trên máy tính này',
    anisette_card_title: '⚙️ Kiểm tra iCloud cho Windows',
    app_subtitle: 'Trình tải và cài đặt tệp IPA App Store cho Windows',
    auth_checking: 'Đang kiểm tra tài khoản...',
    auth_logged_in: 'Đã xác thực',
    auth_not_logged_in: 'Chưa đăng nhập',
    auto_purchase_desc: 'Cần thiết nếu tài khoản Apple ID này chưa từng tải ứng dụng này trước đây (chỉ áp dụng cho ứng dụng miễn phí)',
    auto_purchase_title: 'Tự động nhận bản quyền (Purchase)',
    badge_authenticated: 'Đã xác thực',
    batch_account_label: 'Apple ID dùng cho danh sách này: {email}',
    batch_check_btn: 'Kiểm tra với Apple ID',
    batch_check_item_label: 'Giới hạn thời gian mỗi lần kiểm tra (giây):',
    batch_check_progress_text: 'Đã kiểm tra {done} / {total} (mỗi lượt = {pct}% tiến độ).',
    batch_check_progress_title: 'Đang kiểm tra các ứng dụng qua Tải trực tiếp...',
    batch_check_running: 'Một lượt kiểm tra đang chạy',
    batch_check_start_error: 'Lỗi khởi động kiểm tra hàng loạt',
    batch_desc: 'Tải lên tệp văn bản chứa danh sách ứng dụng (tên và App ID). Mỗi ID sẽ được xử lý qua quy trình Tải trực tiếp: các ứng dụng đã có trong Apple ID sẽ sẵn sàng tải, còn các ứng dụng bạn chưa từng mua (lỗi "license is required") sẽ được lọc ra.',
    batch_download_account_label: 'Tài khoản tải xuống: {email}',
    batch_download_done_title: 'Tải hàng loạt hoàn tất',
    batch_download_progress_done: 'Đã tải {done} / {total}. Lỗi: {errors}.',
    batch_download_progress_title: 'Đang tải hàng loạt các ứng dụng đã chọn...',
    batch_download_selected: 'Tải các mục đã chọn',
    batch_download_single: 'Tải xuống',
    batch_download_skipped_count: 'Đã bỏ qua: {skipped}.',
    batch_download_start_error: 'Lỗi khởi động tải xuống hàng loạt',
    batch_download_started_toast: 'Bắt đầu tải — xem tiến trình bên dưới',
    batch_download_status_completed: 'Hoàn thành',
    batch_download_status_downloading: 'Đang tải xuống',
    batch_download_status_error: 'Lỗi',
    batch_download_status_patching: 'Ký số (sinf)',
    batch_download_status_purchasing: 'Nhận bản quyền',
    batch_download_status_queued: 'Trong hàng đợi',
    batch_download_status_skipped: 'Đã bỏ qua',
    batch_download_status_stalled: 'Treo (đã bỏ qua)',
    batch_file_hint: 'Mỗi dòng một mục: tên và App ID dạng số, ví dụ 1Password 7: 568903335. Có thể dùng ID riêng, liên kết App Store và các dấu phân cách ":", "-", ";", Tab hoặc dấu cách.',
    batch_file_label: 'Tệp danh sách ứng dụng (.txt):',
    batch_filtered_error_title: 'Các lỗi khác:',
    batch_filtered_title: 'Đã lọc ra (chưa có bản quyền trên Apple ID):',
    batch_item_hint: 'Giới hạn tối đa cho mọi giai đoạn của một ứng dụng: nhận bản quyền + tải + ký số.',
    batch_item_label: 'Giới hạn thời gian mỗi ứng dụng (giây):',
    batch_latest_badge: 'Mới nhất',
    batch_need_auth: 'Vui lòng đăng nhập Apple ID trong tab "Tài khoản" trước',
    batch_no_items: 'Không tìm thấy App ID hợp lệ trong danh sách',
    batch_no_selected: 'Vui lòng chọn ít nhất một ứng dụng',
    batch_no_versions: 'Danh sách phiên bản không khả dụng',
    batch_parsed_count: 'Đã nhận diện {count} ứng dụng. Nhấn "Kiểm tra với Apple ID" để quét từng ứng dụng qua Tải trực tiếp.',
    batch_paste_label: 'Hoặc dán danh sách dưới dạng văn bản:',
    batch_paste_placeholder: `1Password 7: 568903335
Telegram: 686449807
568903335 246890568`,
    batch_purchase_hint: 'Khi bật, ứng dụng chưa có bản quyền sẽ được mua trước khi kiểm tra/tải (chỉ dành cho ứng dụng miễn phí). Khi tắt, ứng dụng đó sẽ bị lọc ra và không tải.',
    batch_purchase_label: 'Tự động nhận bản quyền (Purchase)',
    batch_results_summary: 'Khả dụng để tải: {available} / {total}. Đã lọc ra: {filtered}.',
    batch_results_title: 'Kết quả kiểm tra',
    batch_retry_failed: 'Thử lại thất bại',
    batch_retry_running: 'Một lượt thử lại đang chạy',
    batch_save_list: 'Lưu danh sách (.txt)',
    batch_save_toast: 'Đã lưu danh sách vào tệp văn bản',
    batch_select_all: 'Chọn tất cả',
    batch_select_hint: 'Chọn các ứng dụng và nhấn "Tải các mục đã chọn":',
    batch_select_none: 'Bỏ chọn tất cả',
    batch_skip_btn: 'Bỏ qua',
    batch_skip_error: 'Không thể bỏ qua ứng dụng',
    batch_skip_toast: 'Đã bỏ qua ứng dụng — chuyển sang mục tiếp theo',
    batch_stall_hint: 'Nếu ứng dụng không nhận được một byte nào quá thời gian này, ứng dụng sẽ bị đánh dấu là "Treo" và chuyển sang ứng dụng tiếp theo.',
    batch_stall_label: 'Xem là bị treo khi không nhận dữ liệu sau (giây):',
    batch_stalled_hint: 'không nhận dữ liệu trong {sec} giây',
    batch_stalled_status: 'không phản hồi (đã bỏ qua)',
    batch_title: 'Kiểm tra & Tải hàng loạt',
    batch_version_history: 'Lịch sử phiên bản',
    batch_version_history_close: 'Ẩn lịch sử',
    batch_version_pick_hint: 'Đánh dấu phiên bản để tải thay vì bản mới nhất (qua nút "Tải các mục đã chọn" ở trên). Đánh dấu sẽ bỏ chọn ô trên thẻ ứng dụng.',
    batch_versions_loading: 'Đang lấy dữ liệu phiên bản...',
    cancel_btn: 'Hủy',
    close_btn: 'Đóng',
    copied_toast: 'Đã sao chép vào bộ nhớ tạm',
    copy_path_btn: 'Sao chép đường dẫn',
    copy_tooltip: 'Sao chép',
    default_user_name: 'Người dùng Apple',
    direct_desc: 'Tải ứng dụng theo Bundle ID, App ID hoặc liên kết App Store trực tiếp',
    direct_id_hint: 'Bạn có thể cung cấp Bundle Identifier (ví dụ: com.example.app), Track ID dạng số (ví dụ: 686449807), hoặc liên kết web App Store.',
    direct_id_label: 'Định danh ứng dụng (Bundle ID, App ID hoặc URL):',
    direct_id_placeholder: 'ví dụ: org.telegram.Telegram-iOS, 686449807 hoặc https://apps.apple.com/...',
    direct_specify_id: 'Vui lòng nhập Bundle ID hoặc App ID',
    direct_title: 'Tải trực tiếp ứng dụng',
    dl_completed_ipa: 'Đã tải thành công gói .IPA!',
    dl_completed_pkg: 'Đã tải thành công gói .PKG!',
    dl_modal_default_subtitle: 'Đang chuẩn bị tải...',
    dl_modal_default_title: 'Đang tải ứng dụng',
    dl_modal_download_failed: 'Đã xảy ra lỗi trong quá trình tải xuống',
    dl_modal_saved_toast: 'Đã lưu tệp: {path}',
    dl_modal_status_downloading: 'Đang tải gói cài đặt đã mã hóa...',
    dl_modal_status_licensing: 'Đang nhận bản quyền từ App Store...',
    dl_modal_status_patching: 'Đang áp dụng chữ ký số (sinf)...',
    donate_button: 'Ủng hộ dự án qua Cloudtips',
    donate_desc: 'Nếu bạn muốn ủng hộ dự án, bạn có thể thực hiện qua liên kết bên dưới hoặc quét mã QR:',
    donate_qr_caption: 'Mã QR ủng hộ dự án',
    donate_title: 'Ủng hộ dự án',
    download_btn: 'Tải IPA',
    download_btn_pkg: 'Tải PKG',
    download_history_desc: 'Các gói .IPA đã tải hoàn tất trên máy tính của bạn',
    download_history_title: 'Lịch sử tải xuống',
    download_start_error: 'Lỗi khởi động tải xuống',
    enter_2fa_code: 'Vui lòng nhập mã gồm 6 chữ số',
    faq_a1: 'Máy chủ Apple App Store chỉ cấp phát gói cài đặt <code>.ipa</code> cho người dùng đã xác thực. Việc đăng nhập là bắt buộc để Apple tạo chữ ký số cá nhân hóa (<code>sinf</code>) và cấp giấy phép ứng dụng cho tài khoản của bạn.',
    faq_a2: 'Có. Ứng dụng hoàn toàn là mã nguồn mở (Open Source). Thông tin đăng nhập của bạn chỉ được gửi độc quyền tới máy chủ bảo mật chính thức của Apple (<code>auth.itunes.apple.com</code> / <code>gsa.apple.com</code>) bằng giao thức mật mã SRP-6a. Dữ liệu không bao giờ được gửi tới bất kỳ máy chủ bên thứ ba nào.',
    faq_a3: 'Điều này có nghĩa là ứng dụng chưa từng được mua/nhận bằng tài khoản Apple ID này. Bạn chỉ cần nhấn nút <strong>"Nhận bản quyền"</strong> trên thẻ ứng dụng hoặc tích chọn <em>"Tự động nhận bản quyền (Purchase)"</em> khi tải.',
    faq_a4: 'Bạn có thể tải ứng dụng trả phí nếu trước đó tài khoản Apple ID của bạn đã từng mua ứng dụng đó. Tính năng tự động nhận bản quyền miễn phí của ipatool chỉ áp dụng cho các ứng dụng miễn phí trên App Store.',
    faq_a5: 'Chuyển sang tab <strong>"Lịch sử phiên bản"</strong> (hoặc nhấn nút "Phiên bản" trên thẻ ứng dụng). Chương trình sẽ hiển thị tất cả các phiên bản từng phát hành. Chọn bản build mong muốn và nhấn "Tải xuống".',
    faq_a6: 'Mặc định, tệp được lưu vào thư mục <code>apps</code> bên trong thư mục làm việc của chương trình. Bạn có thể mở thư mục bất kỳ lúc nào bằng cách nhấn nút <strong>"📂 Mở thư mục tải xuống"</strong> trong tab "Tải xuống".',
    faq_a7: '<p>Nếu bạn nhận thông báo lỗi <code>invalid response: Apple Media Services Terms and Conditions have changed</code>, điều đó có nghĩa là Apple đã cập nhật điều khoản dịch vụ và tài khoản của bạn chưa chấp nhận các điều khoản mới.</p><p><strong data-i18n="faq_a7_fix_title">Cách khắc phục:</strong></p><ol><li data-i18n="faq_a7_step1">Mở App Store trên iPhone, iPad hoặc máy Mac của bạn</li><li data-i18n="faq_a7_step2">Đăng nhập bằng đúng tài khoản Apple ID đó nếu được nhắc</li><li data-i18n="faq_a7_step3">Bắt đầu tải bất kỳ ứng dụng nào (có thể thử chính ứng dụng vừa bị lỗi, ví dụ Insta360) - thao tác này sẽ kích hoạt hộp thoại điều khoản mới</li><li data-i18n="faq_a7_step4">Chấp nhận các điều khoản và điều kiện mới khi xuất hiện</li><li data-i18n="faq_a7_step5">Sau khi đồng ý điều khoản, ipatool sẽ hoạt động bình thường trở lại</li></ol><p data-i18n="faq_a7_note">Đây là quy trình bắt buộc tiêu chuẩn của Apple khi cập nhật điều khoản pháp lý. Khi bạn đã chấp nhận qua ứng dụng chính thức của Apple, lỗi sẽ được giải quyết.</p>',
    faq_a7_fix_title: 'Cách khắc phục:',
    faq_a7_note: 'Đây là quy trình bắt buộc tiêu chuẩn của Apple khi cập nhật điều khoản pháp lý. Khi bạn đã chấp nhận qua ứng dụng chính thức của Apple, lỗi sẽ được giải quyết.',
    faq_a7_step1: 'Mở App Store trên iPhone, iPad hoặc máy Mac của bạn',
    faq_a7_step2: 'Đăng nhập bằng đúng tài khoản Apple ID đó nếu được nhắc',
    faq_a7_step3: 'Bắt đầu tải bất kỳ ứng dụng nào (có thể thử chính ứng dụng vừa bị lỗi, ví dụ Insta360) - thao tác này sẽ kích hoạt hộp thoại điều khoản mới',
    faq_a7_step4: 'Chấp nhận các điều khoản và điều kiện mới khi xuất hiện',
    faq_a7_step5: 'Sau khi đồng ý điều khoản, ipatool sẽ hoạt động bình thường trở lại',
    faq_q1: 'Tại sao cần phải đăng nhập bằng Apple ID?',
    faq_q2: 'Nhập mật khẩu Apple ID vào ipatool có an toàn không?',
    faq_q3: 'Làm gì nếu khi tải ứng dụng báo lỗi "license required"?',
    faq_q4: 'Tôi có thể tải ứng dụng trả phí không?',
    faq_q5: 'Làm sao để tải phiên bản cũ của ứng dụng?',
    faq_q6: 'Các tệp .IPA sau khi tải được lưu ở đâu?',
    faq_q7: 'Làm gì nếu gặp lỗi "invalid response" khi tải xuống?',
    faq_title: 'Các câu hỏi thường gặp (FAQ)',
    fetch_versions_btn: 'Lấy danh sách phiên bản',
    fetching_versions_status: 'Đang lấy danh sách bản build từ máy chủ Apple...',
    filename_warning: 'Không thể đọc tên ứng dụng và phiên bản từ Info.plist; tên tệp tải về được giữ nguyên.',
    free_price: 'Miễn phí',
    guide_desc: 'Tệp .IPA đã tải là gói cài đặt chính gốc từ App Store. Dưới đây là các phương pháp tốt nhất để cài đặt:',
    guide_title: '📱 Hướng dẫn: Cách cài đặt .IPA lên iPhone hoặc iPad',
    history_refreshed: 'Đã làm mới danh sách',
    icloud_download_btn: 'Tải iCloud cổ điển cho Windows',
    icloud_installed: '✅ Đã cài đặt bản iCloud cổ điển (Classic) cho Windows.',
    icloud_installed_classic: 'Tìm thấy Apple\\Internet Services với AOSKit.dll.',
    icloud_not_installed: '❌ Không tìm thấy bản iCloud cổ điển cho Windows. Tải về và cài đặt qua liên kết bên dưới, sau đó đăng nhập Apple ID của bạn.',
    icloud_store_url: 'https://updates.cdn-apple.com/2020/windows/001-39935-20200911-1A70AA56-F448-11EA-8CC0-99D41950005E/iCloudSetup.exe',
    in_explorer_btn: 'Trong Explorer',
    info_country: 'Vùng App Store (Storefront):',
    info_dsid: 'DSID:',
    info_keychain: 'Lưu trữ phiên (Keychain):',
    info_keychain_ok: 'Keychain hồ sơ riêng biệt',
    install_button: 'Cài đặt lên thiết bị',
    install_completed: 'Ứng dụng đã được cài đặt thành công!',
    install_device_label: 'Thiết bị:',
    install_device_select_placeholder: '— chọn thiết bị —',
    install_devices_desc: 'Tự động phát hiện qua libimobiledevice (idevice_id / idevicedeviceinfo)',
    install_devices_refresh_error: 'Không thể lấy danh sách thiết bị',
    install_devices_scanning: 'Đang tìm kiếm thiết bị kết nối...',
    install_devices_title: 'Thiết bị iOS đã kết nối',
    install_driver_check_title: 'Trình điều khiển Apple Mobile Device Support',
    install_driver_checking: 'Đang kiểm tra trình điều khiển...',
    install_driver_download: 'Tải Apple Mobile Device Support',
    install_driver_itunes: 'Tải iTunes',
    install_driver_missing: 'Không tìm thấy trình điều khiển/dịch vụ Apple Mobile Device Support. Cài đặt iTunes để nhận driver hoặc cập nhật driver USB cho thiết bị.',
    install_driver_ok: 'Đã tìm thấy trình điều khiển Apple Mobile Device Support. Kết nối USB sẵn sàng.',
    install_driver_unknown: 'Không thể kiểm tra driver trên hệ thống này.',
    install_dropzone_text: 'Nhấn để chọn tệp .ipa',
    install_error: 'Lỗi cài đặt',
    install_file_label: 'Tệp .IPA:',
    install_file_selected: 'Tệp đã chọn:',
    install_log_title: 'Nhật ký ideviceinstaller',
    install_needs_device: 'Vui lòng chọn thiết bị đã kết nối',
    install_needs_file: 'Vui lòng chọn tệp .IPA',
    install_no_devices: 'Kết nối iPhone/iPad bằng cáp và nhấn «Làm mới»',
    install_progress_title: 'Đang cài đặt...',
    install_started_toast: 'Bắt đầu cài đặt — xem tiến trình bên dưới',
    install_status_completed: 'Hoàn thành',
    install_status_error: 'Lỗi',
    install_status_installing: 'Đang cài đặt',
    install_status_queued: 'Trong hàng đợi',
    install_tools_missing_desc: 'Để cài đặt cần có ideviceinstaller từ libimobiledevice. Trên macOS: brew install libimobiledevice. Trên Windows: cài đặt iDevice Suite / bản build libimobiledevice và thêm ideviceinstaller.exe vào PATH hoặc thư mục tools.',
    install_tools_missing_title: 'Không tìm thấy ideviceinstaller',
    install_upload_desc: 'Chọn tệp .IPA đã tải về, chọn thiết bị và cài đặt lên iPhone/iPad',
    install_upload_title: 'Chọn tệp .IPA',
    license_already_owned: 'Tài khoản Apple ID của bạn đã có bản quyền ứng dụng này',
    license_btn: 'Bản quyền',
    license_error: 'Lỗi nhận bản quyền ứng dụng',
    license_success: 'Đã nhận bản quyền thành công!',
    limit_label: 'Số kết quả:',
    login_2fa_required: 'Yêu cầu mã xác thực hai yếu tố (2FA)',
    login_anisette_error: 'Lỗi iCloud (anisette): {msg}',
    login_auth_error: 'Lỗi xác thực Apple ID',
    login_btn: 'Thêm Apple ID',
    login_email_label: 'Apple ID (Email):',
    login_fill_fields: 'Vui lòng nhập Email và Mật khẩu',
    login_password_label: 'Mật khẩu Apple ID:',
    login_prompt_desc: 'Thêm Apple ID khác. Các tài khoản được lưu độc lập và có thể chuyển đổi mà không cần đăng nhập lại.',
    login_prompt_title: 'Thêm tài khoản Apple ID',
    login_security_hint: 'Mật khẩu của bạn được gửi trực tiếp đến Apple qua kết nối bảo mật và chỉ lưu trong keychain hồ sơ để ipatool làm mới phiên; không ghi vào accounts.json. Khi có yêu cầu 2FA, mật khẩu chỉ tạm giữ trong bộ nhớ RAM máy chủ tối đa 10 phút.',
    login_submitting: 'Đang đăng nhập...',
    login_success_toast: 'Đăng nhập thành công: {email}',
    logout_btn: 'Xóa tài khoản khỏi ứng dụng',
    min_ios_badge_title: 'Phiên bản iOS tối thiểu được hỗ trợ',
    modal_2fa_desc: 'Nhập mã xác minh gồm 6 chữ số được gửi tới thiết bị Apple hoặc SMS của bạn',
    modal_2fa_title: 'Xác thực hai yếu tố',
    need_account_for_download: 'Cần đăng nhập Apple ID để tải xuống',
    need_account_for_license: 'Cần đăng nhập Apple ID trong tab "Tài khoản" trước',
    no_active_downloads: 'Không có tiến trình tải nào đang chạy',
    no_history_downloads: 'Lịch sử tải xuống trống',
    no_results_desc: 'Thử tinh chỉnh từ khóa tìm kiếm hoặc đổi nền tảng (iPhone / iPad / Mac).',
    no_results_title: 'Không tìm thấy kết quả',
    no_sinf_short: 'không có sinf',
    no_sinf_warning: 'Apple phân phối gói cài đặt không kèm chữ ký số DRM (sinf). Tệp .ipa vẫn được lưu hoàn chỉnh — có thể giải mã hoặc kiểm tra, nhưng sẽ không thể cài đặt trực tiếp lên thiết bị. Thử lại sau hoặc chọn phiên bản khác.',
    open_downloads_folder: 'Mở thư mục tải xuống',
    open_folder_btn: 'Mở',
    open_folder_error: 'Lỗi khi mở thư mục',
    open_folder_failed: 'Không thể mở thư mục',
    open_in_explorer_btn: 'Hiển thị trong Explorer',
    open_output_folder_title: 'Mở thư mục lưu tệp',
    output_folder_label: 'Thư mục lưu tệp:',
    output_folder_placeholder: 'Mặc định: thư mục apps trong thư mục hiện tại',
    pager_all: 'Tất cả',
    pager_first: '« Đầu',
    pager_last: 'Cuối »',
    pager_next: 'Sau ›',
    pager_page_of: 'Trang {page} / {pages}',
    pager_per_page: 'Mỗi trang:',
    pager_prev: '‹ Trước',
    pager_range: '{from}–{to} trong {total}',
    platform_label: 'Nền tảng:',
    popular_searches: 'Phổ biến:',
    purchases_cache_stale_title: 'Bộ nhớ đệm lịch sử mua chưa được cập nhật hơn 7 ngày',
    purchases_col_action: 'Thao tác',
    purchases_col_app: 'Ứng dụng',
    purchases_col_appid: 'App ID',
    purchases_col_bundle: 'Bundle ID',
    purchases_col_date: 'Ngày mua',
    purchases_desc: 'Tất cả ứng dụng thuộc quyền sở hữu của Apple ID này. Danh sách được tải một lần sau khi đăng nhập và có thể làm mới theo yêu cầu.',
    purchases_download: 'Tải xuống',
    purchases_empty: 'Apple ID này chưa có bất kỳ ứng dụng nào',
    purchases_export: 'Lưu danh sách (.txt)',
    purchases_filter_placeholder: 'Lọc theo tên, Bundle ID hoặc App ID',
    purchases_from_cache: 'từ bộ nhớ đệm',
    purchases_load_failed: 'Không thể tải lịch sử mua hàng',
    purchases_loaded_toast: 'Lịch sử mua hàng: {count} ứng dụng',
    purchases_loading: 'Đang tải lịch sử mua hàng từ Apple...',
    purchases_need_auth: 'Đăng nhập Apple ID trong tab "Tài khoản" để xem lịch sử mua hàng',
    purchases_no_match: 'Không tìm thấy kết quả phù hợp với bộ lọc',
    purchases_nothing_to_export: 'Danh sách trống — không có gì để lưu',
    purchases_only_hidden: 'Chỉ ứng dụng ẩn',
    purchases_only_hidden_title: 'Chỉ hiển thị các ứng dụng bị ẩn hoặc đã bị gỡ khỏi App Store theo vùng',
    purchases_os_all: 'Tất cả HĐH',
    purchases_os_label: 'Hệ điều hành:',
    purchases_refresh: 'Làm mới',
    purchases_retry: 'Thử lại',
    purchases_save_failed: 'Lưu danh sách ra tệp thất bại',
    purchases_saved_toast: 'Đã lưu danh sách: {path}',
    purchases_title: 'Lịch sử mua hàng',
    purchases_unknown_app: 'Không có tên',
    purchases_updated_at: 'Đã cập nhật: {time}',
    purchases_versions: 'Phiên bản',
    refresh_btn: 'Làm mới',
    remove_from_list_title: 'Xóa khỏi danh sách',
    removed_badge: 'Đã gỡ khỏi App Store',
    removed_found: 'Tìm thấy {count} ứng dụng đã gỡ',
    requesting_license: 'Đang yêu cầu bản quyền cho {bundleId}...',
    results_found: 'Tìm thấy {count} ứng dụng',
    search_button: 'Tìm kiếm',
    search_desc: 'Nhập tên ứng dụng hoặc dán liên kết App Store',
    search_error: 'Lỗi tìm kiếm trên App Store',
    search_official_title: 'Kết quả App Store',
    search_placeholder: 'Ví dụ: Telegram, Minecraft hoặc https://apps.apple.com/app/id...',
    search_prompt_desc: 'Tìm kiếm ứng dụng iOS/iPadOS/tvOS/macOS bất kỳ, nhận bản quyền miễn phí và tải gói cài đặt .IPA đã ký số',
    search_prompt_title: 'Tìm kiếm ứng dụng',
    search_query_empty: 'Vui lòng nhập từ khóa tìm kiếm',
    search_removed_title: 'Đã gỡ khỏi App Store (có thể tải theo ID)',
    search_title: 'Tìm kiếm trên Apple App Store',
    searching_status: 'Đang tìm kiếm ứng dụng trên Apple App Store...',
    server_connection_error: 'Lỗi kết nối với máy chủ',
    start_download_btn: 'Bắt đầu tải .IPA',
    status_error_label: 'Lỗi',
    step_complete: 'Sẵn sàng để cài đặt',
    step_download: 'Đang tải gói .IPA từ App Store',
    step_download_pkg: 'Đang tải gói .PKG từ App Store',
    step_license: 'Xác minh bản quyền / Nhận bản quyền',
    step_sinf: 'Áp dụng chữ ký số (sao chép sinf)',
    tab_account: 'Tài khoản Apple ID',
    tab_batch: 'Tải hàng loạt',
    tab_direct: 'Tải trực tiếp',
    tab_donate: 'Ủng hộ dự án',
    tab_downloads: 'Tải xuống',
    tab_guide: 'Hướng dẫn & FAQ',
    tab_install: 'Cài đặt lên thiết bị',
    tab_purchases: 'Lịch sử mua hàng',
    tab_search: 'Tìm kiếm ứng dụng',
    tab_versions: 'Lịch sử phiên bản',
    theme_toggle_title: 'Chuyển đổi giao diện sáng/tối',
    update_available_tip: 'Đã có phiên bản mới: {latest} (hiện tại: {current}) — nhấn để mở trang Releases',
    update_pill_new: '⬆️ Mới: {latest}',
    verify_btn: 'Xác minh mã',
    verifying_code: 'Đang xác minh...',
    version_col_action: 'Thao tác',
    version_col_build: 'ID Bản build (Version ID)',
    version_col_date: 'Ngày phát hành',
    version_col_display: 'Phiên bản hiển thị',
    version_col_select: 'Chọn',
    version_current_tip: 'Phiên bản hiện tại: {current}',
    version_history_tooltip: 'Lịch sử phiên bản',
    version_id_label: 'ID phiên bản (tùy chọn):',
    version_id_placeholder: 'Để trống để tải phiên bản mới nhất',
    versions_app_default: 'Ứng dụng',
    versions_btn: 'Phiên bản',
    versions_count_badge: '{count} phiên bản',
    versions_desc: 'Xem toàn bộ bản build trong lịch sử và tải các phiên bản ứng dụng trước đó',
    versions_fetch_failed: 'Không thể lấy danh sách phiên bản của ứng dụng',
    versions_search_placeholder: 'Nhập Bundle ID (ví dụ: org.telegram.Telegram-iOS) hoặc App ID',
    versions_title: 'Lịch sử phiên bản ứng dụng',
    window_title: 'ipatool webGUI — Tải IPA từ App Store'
  }
};

// Translation helper: looks up text with fallback and template interpolation
function t(key, vars = {}) {
  const dict = i18n[state.lang] || i18n.en || i18n.ru;
  let text = (dict && dict[key]) || (i18n.en && i18n.en[key]) || (i18n.ru && i18n.ru[key]) || key;
  if (vars && typeof vars === 'object') {
    Object.keys(vars).forEach(k => {
      text = text.replaceAll(`{${k}}`, vars[k]);
    });
  }
  return text;
}

// Apply UI Localization
function applyLanguage() {
  const dict = i18n[state.lang] || i18n.ru;
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    if (dict[key]) {
      // Use innerHTML for elements that may contain HTML (like FAQ answers)
      const value = dict[key];
      if (value.includes('<')) {
        el.innerHTML = value;
      } else {
        el.textContent = value;
      }
    }
  });

  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    const key = el.getAttribute('data-i18n-placeholder');
    if (dict[key]) el.setAttribute('placeholder', dict[key]);
  });

  document.querySelectorAll('[data-i18n-title]').forEach(el => {
    const key = el.getAttribute('data-i18n-title');
    if (dict[key]) el.setAttribute('title', dict[key]);
  });

  const langLabel = document.getElementById('lang-label');
  if (langLabel) langLabel.textContent = state.lang.toUpperCase();

  // Re-render purchase history so table buttons/meta follow the language.
  if (typeof renderPurchases === 'function') renderPurchases();
  if (typeof renderVersionsPage === 'function' && state.versionsPage) renderVersionsPage();

  // Update account status pill
  updateAccountStatusPill();
  renderBrandVersion();

  // Re-run iCloud presence check so its text follows the selected language.
  checkICloudStatus();
}

function toggleLanguage() {
  const order = ['ru', 'en', 'vi'];
  const currentIndex = order.indexOf(state.lang);
  state.lang = order[(currentIndex + 1) % order.length];
  localStorage.setItem('ipatool_lang', state.lang);
  applyLanguage();
}

// Theme handling
function applyTheme() {
  document.documentElement.setAttribute('data-theme', state.theme);
  const darkIcon = document.getElementById('theme-icon-dark');
  const lightIcon = document.getElementById('theme-icon-light');
  if (darkIcon && lightIcon) {
    if (state.theme === 'dark') {
      darkIcon.style.display = 'block';
      lightIcon.style.display = 'none';
    } else {
      darkIcon.style.display = 'none';
      lightIcon.style.display = 'block';
    }
  }
}

function toggleTheme() {
  state.theme = state.theme === 'dark' ? 'light' : 'dark';
  localStorage.setItem('ipatool_theme', state.theme);
  applyTheme();
}

// Tab Switching
function switchTab(tabName) {
  state.activeTab = tabName;
  document.querySelectorAll('.tab-btn').forEach(btn => {
    btn.classList.toggle('active', btn.getAttribute('data-tab') === tabName);
  });
  document.querySelectorAll('.tab-content').forEach(sec => {
    sec.classList.toggle('active', sec.id === `tab-${tabName}`);
  });

  if (tabName === 'downloads') {
    renderDownloadsTab();
    startActiveDownloadsPolling();
  } else {
    stopActiveDownloadsPolling();
  }

  if (tabName === 'install') {
    refreshInstallDevices();
  }

  if (tabName === 'purchases') {
    // First load normally happens right after sign-in; this covers the case
    // where that request failed or the user opened the tab before it ran.
    if (state.isAuthenticated && !state.purchases.loaded && !state.purchases.loading) {
      refreshPurchases(false);
    } else {
      renderPurchases();
    }
  }
}

// Toast Notifications
function showToast(message, type = 'info') {
  const container = document.getElementById('toast-container');
  if (!container) return;

  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  
  let icon = 'ℹ️';
  if (type === 'success') icon = '✅';
  if (type === 'error') icon = '❌';

  toast.innerHTML = `<span>${icon}</span><span>${message}</span>`;
  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(10px)';
    setTimeout(() => toast.remove(), 250);
  }, 4000);
}

// Helper: Copy to Clipboard
function copyToClipboard(text, msg) {
  navigator.clipboard.writeText(text).then(() => {
    showToast(msg || i18n[state.lang].copied_toast, 'success');
  }).catch(() => {
    showToast('Failed to copy', 'error');
  });
}

// Helper: Parse App Store URL
function parseAppStoreUrl(input) {
  if (!input) return input;
  const match = input.match(/id(\d+)/i);
  if (match && match[1]) {
    return match[1];
  }
  return input.trim();
}

// Helper: Format bytes
function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Helper: Toggle Password Visibility
function togglePasswordVisibility(inputId) {
  const input = document.getElementById(inputId);
  if (input) {
    input.type = input.type === 'password' ? 'text' : 'password';
  }
}

// ==========================================
// API Interaction & Account Management
// ==========================================

// Shows/hides platform-specific pieces of the UI once the server-reported OS
// is known. The iCloud-for-Windows card is hidden on macOS.
function applyPlatformVisibility() {
  const isMac = state.os === 'darwin';

  const icloudCard = document.getElementById('icloud-presence-card');
  if (icloudCard) icloudCard.style.display = isMac ? 'none' : '';
}

async function checkICloudStatus() {
  // The iCloud presence check is a Windows-only feature; skip it on macOS.
  if (state.os === 'darwin') return;

  const textEl = document.getElementById('icloud-status-text');
  const iconEl = document.getElementById('icloud-status-icon');
  const linkEl = document.getElementById('icloud-download-link');
  if (!textEl) return;
  const dict = i18n[state.lang] || i18n.ru;

  try {
    const res = await fetch('/api/icloud/status');
    const data = await res.json();

    if (data.installed) {
      if (iconEl) iconEl.textContent = '✅';
      if (data.variant === 'classic') {
        textEl.textContent = `${dict.icloud_installed} ${dict.icloud_installed_classic}`;
      } else {
        textEl.textContent = dict.icloud_installed;
      }
      if (linkEl) linkEl.style.display = 'none';
    } else {
      if (iconEl) iconEl.textContent = '❌';
      textEl.textContent = dict.icloud_not_installed;
      if (linkEl) {
        linkEl.href = data.downloadUrl || dict.icloud_store_url || 'https://updates.cdn-apple.com/2020/windows/001-39935-20200911-1A70AA56-F448-11EA-8CC0-99D41950005E/iCloudSetup.exe';
        linkEl.style.display = 'inline-flex';
      }
    }
  } catch (err) {
    textEl.textContent = dict.anisette_alert_desc || '—';
  }
}

async function fetchStatus() {
  try {
    const res = await fetch('/api/status');
    const data = await res.json();
    
    state.isAuthenticated = data.authenticated;
    state.account = data.account || null;
    state.os = data.os || null;
    if (data.version) {
      state.version = data.version;
    }

    renderBrandVersion();
    checkAppUpdate();
    applyPlatformVisibility();
    updateAccountUI();
    onAuthStateChanged();
  } catch (err) {
    console.error('Failed to fetch status:', err);
    state.isAuthenticated = false;
    state.account = null;
    updateAccountUI();
  }
}

function cleanWebGUIVersion(raw) {
  if (!raw) return '';
  return String(raw)
    .trim()
    .replace(/^[vV]/, '')
    .replace(/-webgui$/i, '')
    .trim();
}

function compareWebGUIVersionStrings(a, b) {
  const parse = (val) => {
    const clean = cleanWebGUIVersion(val);
    if (!clean || clean.toLowerCase() === 'dev') return null;
    const m = clean.match(/^(\d+(?:\.\d+)*)([a-zA-Z]*)$/);
    if (!m) return null;
    const nums = m[1].split('.').map(n => parseInt(n, 10));
    while (nums.length < 3) nums.push(0);
    return { nums, suffix: (m[2] || '').toLowerCase() };
  };
  const pa = parse(a);
  const pb = parse(b);
  if (!pa || !pb) return 0;
  const len = Math.max(pa.nums.length, pb.nums.length);
  for (let i = 0; i < len; i++) {
    const av = pa.nums[i] || 0;
    const bv = pb.nums[i] || 0;
    if (av !== bv) return av > bv ? 1 : -1;
  }
  if (pa.suffix !== pb.suffix) {
    return pa.suffix > pb.suffix ? 1 : -1;
  }
  return 0;
}

let updateCheckDone = false;

async function checkAppUpdate() {
  if (updateCheckDone) return;
  updateCheckDone = true;

  try {
    const res = await fetch('/api/update-check');
    const data = await res.json();
    if (res.ok && data && data.success) {
      if (data.currentVersion && !state.version) {
        state.version = data.currentVersion;
      }
      if (data.latestVersion) {
        state.updateInfo = {
          latestVersion: cleanWebGUIVersion(data.latestVersion),
          updateAvailable: !!data.updateAvailable,
          releasesUrl: data.releasesUrl || 'https://github.com/pdx15/ipatool-webGUI/releases'
        };
        renderBrandVersion();
        return;
      }
    }
  } catch (_) {
    // Fall back to direct GitHub API check below
  }

  try {
    const ghRes = await fetch('https://api.github.com/repos/pdx15/ipatool-webGUI/releases/latest');
    if (!ghRes.ok) return;
    const ghData = await ghRes.json();
    const latestClean = cleanWebGUIVersion(ghData.tag_name || ghData.name || '');
    const currentClean = cleanWebGUIVersion(state.version || '');
    if (latestClean) {
      state.updateInfo = {
        latestVersion: latestClean,
        updateAvailable: !!(currentClean && currentClean.toLowerCase() !== 'dev' && compareWebGUIVersionStrings(latestClean, currentClean) > 0),
        releasesUrl: ghData.html_url || 'https://github.com/pdx15/ipatool-webGUI/releases'
      };
      renderBrandVersion();
    }
  } catch (_) {
    // Ignore offline / rate-limit errors
  }
}

function renderBrandVersion() {
  const verEl = document.getElementById('brand-version');
  const pillEl = document.getElementById('brand-update-pill');
  if (!verEl) return;

  const displayVer = cleanWebGUIVersion(state.version || '');
  if (!displayVer) {
    verEl.style.display = 'none';
    if (pillEl) pillEl.style.display = 'none';
    return;
  }

  const releasesUrl = (state.updateInfo && state.updateInfo.releasesUrl) || 'https://github.com/pdx15/ipatool-webGUI/releases';
  verEl.textContent = displayVer;
  verEl.href = releasesUrl;
  verEl.style.display = 'inline-block';

  const hasUpdate = !!(state.updateInfo && state.updateInfo.updateAvailable && state.updateInfo.latestVersion);
  verEl.classList.toggle('has-update', hasUpdate);

  if (hasUpdate) {
    const latest = state.updateInfo.latestVersion;
    const tip = state.lang === 'en'
      ? `New version available: ${latest} (current: ${displayVer}) — click to open Releases`
      : `Доступна новая версия: ${latest} (текущая: ${displayVer}) — нажмите, чтобы открыть страницу релизов`;
    verEl.title = tip;
    if (pillEl) {
      pillEl.href = releasesUrl;
      pillEl.title = tip;
      pillEl.textContent = state.lang === 'en' ? `⬆️ New: ${latest}` : `⬆️ Новая: ${latest}`;
      pillEl.style.display = 'inline-flex';
    }
  } else {
    verEl.title = state.lang === 'en'
      ? `Current version: ${displayVer}`
      : `Текущая версия: ${displayVer}`;
    if (pillEl) {
      pillEl.style.display = 'none';
    }
  }
}

function updateAccountStatusPill() {
  const pill = document.getElementById('account-status-pill');
  const text = document.getElementById('account-status-text');
  if (!pill || !text) return;

  const dict = i18n[state.lang] || i18n.ru;

  if (state.isAuthenticated && state.account) {
    pill.className = 'account-pill logged-in';
    const name = state.account.name || state.account.email.split('@')[0];
    text.textContent = `🟢 ${name} (${state.account.email})`;
  } else {
    pill.className = 'account-pill logged-out';
    text.textContent = `🔴 ${dict.auth_not_logged_in}`;
  }
}

function updateAccountUI() {
  updateAccountStatusPill();

  const loggedInBox = document.getElementById('account-logged-in-box');
  const loggedOutBox = document.getElementById('account-logged-out-box');

  if (state.isAuthenticated && state.account) {
    if (loggedInBox) loggedInBox.style.display = 'block';
    if (loggedOutBox) loggedOutBox.style.display = 'none';

    const nameEl = document.getElementById('account-user-name');
    const emailEl = document.getElementById('account-user-email');
    const countryEl = document.getElementById('account-country');
    const dsidEl = document.getElementById('account-dsid');
    const initialsEl = document.getElementById('account-initials');

    if (nameEl) nameEl.textContent = state.account.name || 'Пользователь Apple';
    if (emailEl) emailEl.textContent = state.account.email;
    if (countryEl) countryEl.textContent = state.account.storefront || 'Apple App Store';
    if (dsidEl) dsidEl.textContent = state.account.dsid || '—';
    if (initialsEl) {
      const char = (state.account.name ? state.account.name[0] : state.account.email[0]).toUpperCase();
      initialsEl.textContent = char;
    }
  } else {
    if (loggedInBox) loggedInBox.style.display = 'none';
    if (loggedOutBox) loggedOutBox.style.display = 'block';
  }
}

// Handle Login
async function handleLogin(e) {
  e.preventDefault();
  const email = document.getElementById('login-email').value.trim();
  const password = document.getElementById('login-password').value;
  const submitBtn = document.getElementById('login-submit-btn');

  if (!email || !password) {
    showToast('Заполните Email и Пароль', 'error');
    return;
  }

  const originalHtml = submitBtn.innerHTML;
  submitBtn.disabled = true;
  submitBtn.textContent = 'Авторизация...';

  await submitLogin('/api/auth/login', email, password, '', submitBtn, originalHtml);
}

// Login submission helper (reused for 2FA retry).
async function submitLogin(endpoint, email, password, authCode, submitBtn, originalHtml) {
  try {
    const res = await fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password, authCode })
    });
    const data = await res.json();

    if (data.authCodeRequired) {
      // 2FA required! Open 2FA modal, remembering which flow triggered it.
      state.lastPendingLogin = { email, password, endpoint };
      open2FAModal();
      showToast('Требуется код двухфакторной аутентификации', 'info');
    } else if (data.anisetteRequired) {
      // Windows GSA login needs a locally installed & signed-in iCloud to
      // produce anisette headers. Show the precise reason returned by the
      // backend so the user knows exactly which check failed.
      showToast('Ошибка iCloud (anisette): ' + (data.message || 'проверьте установку iCloud'), 'error');
      switchTab('account');
    } else if (data.success) {
      close2FAModal();
      state.isAuthenticated = true;
      state.account = data.account;
      updateAccountUI();
      onAuthStateChanged();
      showToast(`Успешный вход: ${data.account.email}`, 'success');
    } else {
      showToast(data.message || 'Ошибка авторизации Apple ID', 'error');
    }
  } catch (err) {
    showToast('Ошибка сетевого соединения с локальным сервером', 'error');
  } finally {
    if (submitBtn) {
      submitBtn.disabled = false;
      if (originalHtml) submitBtn.innerHTML = originalHtml;
    }
    applyLanguage();
  }
}

// 2FA Modal Handlers
function open2FAModal() {
  const modal = document.getElementById('two-factor-modal');
  const input = document.getElementById('two-factor-code');
  if (modal) {
    modal.style.display = 'flex';
    if (input) {
      input.value = '';
      input.focus();
    }
  }
}

function close2FAModal() {
  const modal = document.getElementById('two-factor-modal');
  if (modal) modal.style.display = 'none';
  state.lastPendingLogin = null;
}

async function handle2FASubmit(e) {
  e.preventDefault();
  const code = document.getElementById('two-factor-code').value.trim();
  const submitBtn = document.getElementById('two-factor-submit-btn');

  if (!code || code.length < 6) {
    showToast('Введите 6-значный код', 'error');
    return;
  }

  if (!state.lastPendingLogin) {
    close2FAModal();
    return;
  }

  const originalHtml = submitBtn.innerHTML;
  submitBtn.disabled = true;
  submitBtn.textContent = 'Проверка...';

  // Reuse the same login flow that triggered the 2FA prompt (standard or the
  // macOS MZFinance test path), so the retry does not switch flows.
  const endpoint = state.lastPendingLogin.endpoint || '/api/auth/login';
  await submitLogin(
    endpoint,
    state.lastPendingLogin.email,
    state.lastPendingLogin.password,
    code,
    submitBtn,
    originalHtml
  );
}

// Handle Logout / Revoke
async function confirmLogout() {
  if (!confirm('Вы действительно хотите выйти из учетной записи Apple ID?')) return;

  try {
    const res = await fetch('/api/auth/revoke', { method: 'POST' });
    const data = await res.json();
    if (data.success) {
      state.isAuthenticated = false;
      state.account = null;
      updateAccountUI();
      onAuthStateChanged();
      showToast('Вы успешно вышли из аккаунта', 'success');
    } else {
      showToast(data.message || 'Ошибка выхода', 'error');
    }
  } catch (err) {
    showToast('Ошибка запроса выхода', 'error');
  }
}

// ==========================================
// App Search & Browse
// ==========================================

function quickSearch(term) {
  const input = document.getElementById('search-input');
  if (input) {
    input.value = term;
    handleSearch(new Event('submit'));
  }
}

function clearSearch() {
  const input = document.getElementById('search-input');
  const clearBtn = document.getElementById('search-clear-btn');
  if (input) {
    input.value = '';
    input.focus();
  }
  if (clearBtn) clearBtn.style.display = 'none';
  document.getElementById('search-results-wrapper').style.display = 'none';
  document.getElementById('search-empty').style.display = 'block';
  document.getElementById('search-no-results').style.display = 'none';
}

// Fetches both result groups for a search: the apps that were removed from the
// App Store but stay downloadable by ID (the Apps_ID_List.txt catalog) and the
// official App Store results. /api/search/all returns them in the order the UI
// renders them — removed apps first — so the grouping does not depend on which
// of two parallel responses happens to arrive earlier. If the merged endpoint
// is unavailable (older backend), fall back to the two separate endpoints.
async function fetchSearchResults(term, platform, limit) {
  const termParam = `term=${encodeURIComponent(term)}`;
  try {
    const res = await fetch(`/api/search/all?${termParam}&platform=${platform}&limit=${limit}`);
    if (res.ok) {
      const data = await res.json();
      if (data && data.success) {
        return {
          apps: data.official?.results || [],
          removedApps: data.removed?.results || [],
          message: data.official?.error || ''
        };
      }
    }
  } catch (e) { /* merged endpoint unavailable — use the separate ones */ }

  const [searchRes, removedRes] = await Promise.all([
    fetch(`/api/search?${termParam}&platform=${platform}&limit=${limit}`),
    fetch(`/api/removed-apps?${termParam}&limit=${limit}`)
  ]);
  const data = await searchRes.json();
  if (!data.success) {
    return { apps: [], removedApps: [], message: data.message || 'Ошибка поиска в App Store' };
  }

  let removedData = { success: false, results: [] };
  try {
    removedData = await removedRes.json();
  } catch (e) { /* removed list endpoint unavailable — treat as empty */ }

  return {
    apps: data.results || [],
    removedApps: (removedData && removedData.success) ? (removedData.results || []) : [],
    message: ''
  };
}

async function handleSearch(e) {
  if (e) e.preventDefault();
  let term = document.getElementById('search-input').value.trim();
  const platform = document.getElementById('search-platform').value;
  const limit = document.getElementById('search-limit').value;

  if (!term) {
    showToast('Введите поисковый запрос', 'info');
    return;
  }

  // Check if it's an App Store URL
  term = parseAppStoreUrl(term);

  const loadingEl = document.getElementById('search-loading');
  const emptyEl = document.getElementById('search-empty');
  const noResultsEl = document.getElementById('search-no-results');
  const resultsWrapper = document.getElementById('search-results-wrapper');
  const resultsGrid = document.getElementById('search-results-grid');
  const resultsCount = document.getElementById('results-count');
  const clearBtn = document.getElementById('search-clear-btn');

  if (clearBtn) clearBtn.style.display = 'block';
  emptyEl.style.display = 'none';
  noResultsEl.style.display = 'none';
  resultsWrapper.style.display = 'none';
  loadingEl.style.display = 'block';

  try {
    const { apps, removedApps, message } = await fetchSearchResults(term, platform, limit);

    loadingEl.style.display = 'none';

    if (message) {
      showToast(message, 'error');
      if (removedApps.length === 0) {
        emptyEl.style.display = 'block';
        return;
      }
    }

    const officialSection = document.getElementById('official-results-section');
    const officialHeader = document.getElementById('official-results-header');
    const removedSection = document.getElementById('removed-results-section');
    const removedGrid = document.getElementById('removed-results-grid');
    const removedCount = document.getElementById('removed-results-count');

    resultsGrid.innerHTML = '';
    removedGrid.innerHTML = '';

    // Removed apps are rendered first (their section comes before the official
    // one in index.html), so they are the first thing you see after a search.
    if (removedApps.length > 0) {
      removedSection.style.display = 'block';
      removedCount.textContent = batchText('removed_found', { count: removedApps.length });
      removedApps.forEach(app => removedGrid.appendChild(createRemovedAppCard(app, platform)));
    } else {
      removedSection.style.display = 'none';
    }

    if (apps.length > 0) {
      officialSection.style.display = 'block';
      resultsCount.textContent = batchText('results_found', { count: apps.length });
      apps.forEach(app => resultsGrid.appendChild(createAppCard(app, platform)));
    } else {
      officialSection.style.display = 'none';
    }

    // The divider only makes sense when both groups are on screen; otherwise
    // the official results would start with a stray horizontal rule.
    if (officialHeader) {
      officialHeader.classList.toggle('results-header-divider', removedApps.length > 0 && apps.length > 0);
    }

    if (apps.length === 0 && removedApps.length === 0) {
      noResultsEl.style.display = 'block';
      resultsWrapper.style.display = 'none';
      return;
    }

    resultsWrapper.style.display = 'block';
  } catch (err) {
    loadingEl.style.display = 'none';
    showToast('Ошибка связи с сервером', 'error');
    emptyEl.style.display = 'block';
  }
}

function createAppCard(app, platform) {
  const card = document.createElement('div');
  card.className = 'app-card';

  const iconUrl = app.artworkUrl512 || app.artworkUrl100 || app.artworkUrl60;
  const priceText = app.price === 0 || !app.price ? (i18n[state.lang]?.free_price || 'Бесплатно') : (app.formattedPrice || `$${app.price}`);
  const dict = i18n[state.lang] || i18n.ru;
  const dlBtnText = platformOSLabel(platform) === 'macOS' ? (dict.download_btn_pkg || dict.download_btn) : dict.download_btn;

  card.innerHTML = `
    <div class="app-main-info">
      ${iconUrl ? `<img src="${iconUrl}" alt="${app.trackName}" class="app-icon-img" loading="lazy" onerror="this.outerHTML='<div class=\\'app-icon-fallback\\'>📱</div>'">` : `<div class="app-icon-fallback">📱</div>`}
      <div class="app-meta">
        <h3 class="app-name" title="${app.trackName}">${app.trackName || 'Без названия'}</h3>
        <p class="app-artist" title="${app.artistName || app.sellerName || ''}">${app.artistName || app.sellerName || 'Apple Developer'}</p>
        <div class="app-badges">
          <span class="badge badge-price">${priceText}</span>
          ${app.version ? `<span class="badge badge-version">v${app.version}</span>` : ''}
          ${app.fileSizeBytes ? `<span class="badge">${formatBytes(app.fileSizeBytes)}</span>` : ''}
          <span class="badge">${platform.toUpperCase()}</span>
        </div>
      </div>
    </div>

    <div class="app-identifiers">
      <div class="id-row">
        <span class="id-label">Bundle ID:</span>
        <span class="id-value">
          <code>${app.bundleId}</code>
          <button type="button" class="copy-icon-btn" onclick="copyToClipboard('${app.bundleId}')" title="Скопировать">📋</button>
        </span>
      </div>
      <div class="id-row">
        <span class="id-label">App ID:</span>
        <span class="id-value">
          <code>${app.trackId}</code>
          <button type="button" class="copy-icon-btn" onclick="copyToClipboard('${app.trackId}')" title="Скопировать">📋</button>
        </span>
      </div>
    </div>

    <div class="app-actions">
      <button class="btn btn-primary" onclick="startAppDownload('${app.bundleId}', ${app.trackId}, '${platform}', '${app.trackName.replace(/'/g, "\\'")}', '${iconUrl || ''}')">
        ⬇️ ${dlBtnText}
      </button>
      <button class="btn btn-outline" onclick="purchaseApp('${app.bundleId}', '${platform}')" title="Получить лицензию">
        🛒 ${dict.license_btn}
      </button>
      <button class="btn btn-outline" onclick="viewAppVersions('${app.bundleId}', ${app.trackId}, '${app.trackName.replace(/'/g, "\\'")}', '${platform}')" title="История версий">
        🕒 ${dict.versions_btn}
      </button>
    </div>
  `;

  return card;
}

// Renders a card for an app that was removed from the App Store but is still
// downloadable by its numeric App ID (from the Apps_ID_List.txt catalog).
// Unlike official results there is no artwork or bundle ID, so only App ID
// based actions (download / version history) are offered.
function createRemovedAppCard(app, platform) {
  const card = document.createElement('div');
  card.className = 'app-card removed-app-card';

  const dict = i18n[state.lang] || i18n.ru;
  const dlBtnText = platformOSLabel(platform) === 'macOS' ? (dict.download_btn_pkg || dict.download_btn) : dict.download_btn;
  const name = app.name || String(app.appId);
  const nameEsc = String(name).replace(/'/g, "\\'");
  const appId = Number(app.appId) || 0;

  card.innerHTML = `
    <div class="app-main-info">
      <div class="app-icon-fallback removed-icon">🗄️</div>
      <div class="app-meta">
        <h3 class="app-name" title="${batchEscapeHtml(name)}">${batchEscapeHtml(name)}</h3>
        <p class="app-artist">App ID: <code>${appId}</code></p>
        <div class="app-badges">
          <span class="badge badge-removed">${dict.removed_badge}</span>
          <span class="badge">${platform.toUpperCase()}</span>
        </div>
      </div>
    </div>

    <div class="app-identifiers">
      <div class="id-row">
        <span class="id-label">App ID:</span>
        <span class="id-value">
          <code>${appId}</code>
          <button type="button" class="copy-icon-btn" onclick="copyToClipboard('${appId}')" title="Скопировать">📋</button>
        </span>
      </div>
    </div>

    <div class="app-actions">
      <button class="btn btn-primary" onclick="startAppDownload('', ${appId}, '${platform}', '${nameEsc}', '')">
        ⬇️ ${dlBtnText}
      </button>
      <button class="btn btn-outline" onclick="viewAppVersions('', ${appId}, '${nameEsc}', '${platform}')" title="История версий">
        🕒 ${dict.versions_btn}
      </button>
    </div>
  `;

  return card;
}

// Purchase App License
async function purchaseApp(bundleId, platform = 'iphone') {
  if (!state.isAuthenticated) {
    showToast('Сначала необходимо войти в Apple ID во вкладке "Аккаунт"', 'error');
    switchTab('account');
    return;
  }

  showToast(`Запрос лицензии для ${bundleId}...`, 'info');

  try {
    const res = await fetch('/api/purchase', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ bundleId, platform })
    });
    const data = await res.json();

    if (data.success) {
      if (data.alreadyOwned) {
        showToast('Лицензия уже имеется на вашем Apple ID', 'success');
      } else {
        showToast('Лицензия успешно получена!', 'success');
      }
    } else {
      showToast(data.message || 'Ошибка приобретения лицензии', 'error');
    }
  } catch (err) {
    showToast('Ошибка запроса к серверу', 'error');
  }
}

// ==========================================
// Download Execution & Real-Time Progress
// ==========================================

async function startAppDownload(bundleId, appId, platform, appName, iconUrl, versionId = '', outputPath = '') {
  if (!state.isAuthenticated) {
    showToast('Для скачивания необходимо войти в Apple ID', 'error');
    switchTab('account');
    return;
  }

  // Prevent double-click
  if (state.isDownloading) {
    return;
  }
  state.isDownloading = true;

  // Open download progress modal
  openDownloadModal(appName, bundleId, iconUrl, platform);

  try {
    const res = await fetch('/api/download', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        bundleId: bundleId || '',
        appId: appId ? parseInt(appId, 10) : 0,
        appName: appName || '',
        platform: platform || 'iphone',
        externalVersionId: versionId || '',
        outputPath: outputPath || '',
        purchase: true // auto purchase enabled
      })
    });
    const data = await res.json();

    if (!data.success) {
      state.isDownloading = false;
      updateDownloadModalError(data.message || 'Ошибка запуска скачивания');
      return;
    }

    state.currentJobId = data.jobId;
    trackDownloadProgress(data.jobId, appName, bundleId, iconUrl, platform);
  } catch (err) {
    state.isDownloading = false;
    updateDownloadModalError('Ошибка запуска загрузки');
  }
}

function updateDownloadModalStepLabels(platform) {
  const isMac = platformOSLabel(platform) === 'macOS';
  const stepDownloadText = document.querySelector('#step-download .step-text');
  if (stepDownloadText) {
    const key = isMac ? 'step_download_pkg' : 'step_download';
    stepDownloadText.setAttribute('data-i18n', key);
    stepDownloadText.textContent = batchText(key);
  }
}

function openDownloadModal(title, subtitle, iconUrl, platform = 'iphone') {
  const modal = document.getElementById('download-modal');
  const titleEl = document.getElementById('dl-modal-title');
  const subEl = document.getElementById('dl-modal-subtitle');
  const iconEl = document.getElementById('dl-modal-icon');
  const fillEl = document.getElementById('dl-progress-bar-fill');
  const percentEl = document.getElementById('dl-progress-percent');
  const bytesEl = document.getElementById('dl-progress-bytes');
  const speedEl = document.getElementById('dl-progress-speed');
  const errorBox = document.getElementById('dl-error-box');
  const openBtn = document.getElementById('dl-modal-open-btn');

  if (titleEl) titleEl.textContent = title || 'Скачивание приложения';
  if (subEl) subEl.textContent = subtitle || 'Подготовка...';
  if (iconEl) {
    if (iconUrl) {
      iconEl.innerHTML = `<img src="${iconUrl}" style="width:64px;height:64px;border-radius:14px;">`;
    } else {
      iconEl.textContent = '📦';
    }
  }

  if (fillEl) fillEl.style.width = '0%';
  if (percentEl) percentEl.textContent = '0%';
  if (bytesEl) bytesEl.textContent = '0 / 0 MB';
  if (speedEl) speedEl.textContent = '—';
  if (errorBox) errorBox.style.display = 'none';
  updateDownloadModalWarning('');
  if (openBtn) openBtn.style.display = 'none';

  // Reset steps
  document.querySelectorAll('.dl-step').forEach(step => step.className = 'dl-step');
  updateDownloadModalStepLabels(platform);
  const stepLicense = document.getElementById('step-license');
  if (stepLicense) stepLicense.classList.add('active');

  if (modal) modal.style.display = 'flex';
}

// Maps a server-side job warning to the localised text shown in the UI.
function localizeJobWarning(warning) {
  if (!warning) return '';
  return String(warning).split(/\n+/).filter(Boolean).map((item) => {
    if (item === 'ipatool-gui-filename-fallback') return batchText('filename_warning');
    if (/sinf/i.test(item)) return batchText('no_sinf_warning');
    return item;
  }).join(' ');
}

function updateDownloadModalWarning(warning) {
  const box = document.getElementById('dl-warning-box');
  const text = document.getElementById('dl-warning-text');
  if (!box || !text) return;
  const msg = localizeJobWarning(warning);
  text.textContent = msg;
  box.style.display = msg ? 'block' : 'none';
}

function updateDownloadModalError(errMsg) {
  const errorBox = document.getElementById('dl-error-box');
  const errorText = document.getElementById('dl-error-text');
  if (errorBox && errorText) {
    errorText.textContent = errMsg;
    errorBox.style.display = 'block';
  }
}

function closeDownloadModal() {
  const modal = document.getElementById('download-modal');
  if (modal) modal.style.display = 'none';
}

function trackDownloadProgress(jobId, appName, bundleId, iconUrl, platform = 'iphone') {
  const fillEl = document.getElementById('dl-progress-bar-fill');
  const percentEl = document.getElementById('dl-progress-percent');
  const bytesEl = document.getElementById('dl-progress-bytes');
  const speedEl = document.getElementById('dl-progress-speed');
  const subEl = document.getElementById('dl-modal-subtitle');
  const openBtn = document.getElementById('dl-modal-open-btn');

  let handling = false; // Prevent parallel callback execution

  const interval = setInterval(async () => {
    // Skip if already handling or job already fully processed
    if (handling || state.completedJobIds.has(jobId)) return;
    handling = true;

    try {
      const res = await fetch(`/api/download/status?jobId=${jobId}`);
      const job = await res.json();

      if (!job.id) {
        clearInterval(interval);
        handling = false;
        return;
      }

      const effectivePlatform = job.platform || platform || 'iphone';
      const isMac = platformOSLabel(effectivePlatform) === 'macOS' || /\.pkg$/i.test(job.outputPath || '');
      updateDownloadModalStepLabels(isMac ? 'macos' : effectivePlatform);

      // Update state
      state.activeDownloads.set(jobId, job);
      updateDownloadsBadge();
      renderActiveDownloadsTab();

      // Update modal progress bar
      const pct = Math.min(100, Math.max(0, job.progress || 0));
      if (fillEl) fillEl.style.width = `${pct}%`;
      if (percentEl) percentEl.textContent = `${pct.toFixed(1)}%`;
      if (bytesEl) {
        bytesEl.textContent = job.totalBytes > 0
          ? `${formatBytes(job.bytesRead)} / ${formatBytes(job.totalBytes)}`
          : `${formatBytes(job.bytesRead)}`;
      }
      if (speedEl) speedEl.textContent = job.speed || '';

      // Update steps
      const stepLic = document.getElementById('step-license');
      const stepDl = document.getElementById('step-download');
      const stepSinf = document.getElementById('step-sinf');
      if (job.status === 'purchasing' || job.status === 'queued') {
        if (stepLic) {
          stepLic.classList.remove('completed');
          stepLic.classList.add('active');
        }
        if (stepDl) stepDl.classList.remove('active', 'completed');
        if (stepSinf) stepSinf.classList.remove('active', 'completed');
        if (subEl) subEl.textContent = 'Получение лицензии App Store...';
      } else if (job.status === 'downloading') {
        if (stepLic) {
          stepLic.classList.remove('active');
          stepLic.classList.add('completed');
        }
        if (stepDl) {
          stepDl.classList.remove('completed');
          stepDl.classList.add('active');
        }
        if (stepSinf) stepSinf.classList.remove('active', 'completed');
        if (subEl) subEl.textContent = 'Скачивание зашифрованного пакета...';
      } else if (job.status === 'patching') {
        if (stepLic) {
          stepLic.classList.remove('active');
          stepLic.classList.add('completed');
        }
        if (stepDl) {
          stepDl.classList.remove('active');
          stepDl.classList.add('completed');
        }
        if (stepSinf) {
          stepSinf.classList.remove('completed');
          stepSinf.classList.add('active');
        }
        if (subEl) subEl.textContent = 'Применение цифровой подписи (sinf)...';
      } else if (job.status === 'completed') {
        clearInterval(interval);

        // Skip if already processed (prevents duplicate toasts from parallel callbacks)
        if (state.completedJobIds.has(jobId)) {
          handling = false;
          return;
        }
        state.completedJobIds.add(jobId);

        document.getElementById('step-sinf')?.classList.add('completed');
        document.getElementById('step-complete')?.classList.add('completed');
        if (subEl) subEl.textContent = batchText(isMac ? 'dl_completed_pkg' : 'dl_completed_ipa');
        if (fillEl) fillEl.style.width = '100%';
        if (percentEl) percentEl.textContent = '100%';
        if (openBtn) {
          openBtn.style.display = 'inline-flex';
          openBtn.setAttribute('data-path', job.outputPath || '');
        }

        addToHistory({
          appName: job.appName || appName || bundleId,
          bundleId: job.bundleId || bundleId,
          version: job.version || '',
          outputPath: job.outputPath,
          bytes: job.totalBytes,
          date: new Date().toLocaleString()
        });

        state.activeDownloads.delete(jobId);
        state.isDownloading = false;
        updateDownloadsBadge();
        renderActiveDownloadsTab();
        if (job.warning) {
          updateDownloadModalWarning(job.warning);
          showToast(localizeJobWarning(job.warning), 'warning');
        } else {
          showToast(`Файл сохранен: ${job.outputPath}`, 'success');
        }
        handling = false;
        return;
      } else if (job.status === 'error') {
        clearInterval(interval);
        updateDownloadModalError(job.error || 'Произошла ошибка при скачивании');
        state.activeDownloads.delete(jobId);
        state.isDownloading = false;
        updateDownloadsBadge();
        renderActiveDownloadsTab();
        handling = false;
        return;
      }

      handling = false;
    } catch (err) {
      console.error('Progress poll error:', err);
      handling = false;
    }
  }, 1000);
}

function openCompletedFolder() {
  const openBtn = document.getElementById('dl-modal-open-btn');
  const path = openBtn ? openBtn.getAttribute('data-path') : '';
  openOutputFolder(path);
}

// ==========================================
// Direct Download Form
// ==========================================

async function handleDirectDownload(e) {
  e.preventDefault();
  const rawId = document.getElementById('direct-app-id').value.trim();
  const platform = document.getElementById('direct-platform').value;
  const versionId = document.getElementById('direct-version-id').value.trim();
  const outputPath = document.getElementById('direct-output-path').value.trim();
  const purchase = document.getElementById('direct-purchase').checked;

  if (!rawId) {
    showToast('Укажите Bundle ID или App ID', 'error');
    return;
  }

  const parsedId = parseAppStoreUrl(rawId);
  const isNumeric = /^\d+$/.test(parsedId);

  const bundleId = isNumeric ? '' : parsedId;
  const appId = isNumeric ? parsedId : 0;

  await startAppDownload(bundleId, appId, platform, parsedId, '', versionId, outputPath);
}

// ==========================================
// Version History Tab
// ==========================================

async function viewAppVersions(bundleId, appId, appName, platform = 'iphone') {
  switchTab('versions');
  // Remember the app name and numeric ID so /api/versions and metadata batches
  // do not need to re-run Lookup when opened from an existing app card.
  state.lastVersionsName = appName || '';
  state.lastVersionsAppId = Number(appId) || 0;
  state.lastVersionsInput = String(bundleId || appId || '').trim();
  const platformSelect = document.getElementById('versions-platform');
  if (platformSelect && platform) {
    platformSelect.value = platform;
  }
  const input = document.getElementById('versions-input');
  if (input) {
    input.value = bundleId || appId;
    handleFetchVersions(new Event('submit'));
  }
}

// Fetches the Display Version and Release Date for a single build and fills
// the corresponding cells in the version history table.
async function fetchVersionMetadata(bundleId, appId, versionId, platform = 'iphone') {
  try {
    const res = await fetch(`/api/version-metadata?bundleId=${encodeURIComponent(bundleId)}&appId=${encodeURIComponent(appId)}&versionId=${encodeURIComponent(versionId)}&platform=${encodeURIComponent(platform)}`);
    const data = await res.json();
    const dispEl = document.getElementById(`disp-ver-${versionId}`);
    const dateEl = document.getElementById(`date-ver-${versionId}`);
    if (!dispEl && !dateEl) return;

    if (data.success) {
      if (dispEl) dispEl.innerHTML = renderDisplayVersionCell(data.displayVersion, data.minimumOSVersion, platform);
      if (dateEl) dateEl.textContent = data.releaseDate || '—';
    } else {
      if (dispEl) dispEl.textContent = '—';
      if (dateEl) dateEl.textContent = '—';
    }
  } catch (err) {
    const dispEl = document.getElementById(`disp-ver-${versionId}`);
    const dateEl = document.getElementById(`date-ver-${versionId}`);
    if (dispEl) dispEl.textContent = '—';
    if (dateEl) dateEl.textContent = '—';
  }
}

function platformOSLabel(platform) {
  switch (String(platform || '').toLowerCase()) {
    case 'ipad':
    case 'ipados':
      return 'iPadOS';
    case 'appletv':
    case 'apple-tv':
    case 'tvos':
      return 'tvOS';
    case 'watch':
    case 'watchos':
    case 'applewatch':
    case 'apple-watch':
      return 'watchOS';
    case 'vision':
    case 'visionos':
    case 'visionpro':
    case 'xros':
    case 'realitydevice':
      return 'visionOS';
    case 'mac':
    case 'macos':
    case 'osx':
      return 'macOS';
    default:
      return 'iOS';
  }
}

// Builds the inner HTML of a "Display Version" table cell: the app version and,
// when known, a small OS badge showing the minimum supported OS version from
// the selected build's Info.plist / package metadata.
function renderDisplayVersionCell(displayVersion, minimumOSVersion, platform = 'iphone') {
  const version = batchEscapeHtml(displayVersion || '—');
  const minOS = String(minimumOSVersion || '').trim();
  if (!minOS) return version;
  const osLabel = platformOSLabel(platform);
  const title = batchText('min_ios_badge_title').replace(/\biOS\b/g, osLabel);
  return `${version} <span class="badge badge-ios" title="${batchEscapeHtml(title)}">${batchEscapeHtml(osLabel)} ${batchEscapeHtml(minOS)}</span>`;
}

async function handleFetchVersions(e) {
  e.preventDefault();
  const rawInput = document.getElementById('versions-input').value.trim();
  if (!rawInput) return;

  const parsed = parseAppStoreUrl(rawInput);
  const isNumeric = /^\d+$/.test(parsed);

  const matchedPreset = state.lastVersionsInput && (rawInput === state.lastVersionsInput || parsed === state.lastVersionsInput);
  const presetName = matchedPreset ? (state.lastVersionsName || '') : '';
  const presetAppId = matchedPreset ? (Number(state.lastVersionsAppId) || 0) : 0;
  if (!matchedPreset) {
    state.lastVersionsName = '';
    state.lastVersionsAppId = 0;
    state.lastVersionsInput = '';
  }

  const bundleId = isNumeric ? '' : parsed;
  const appId = isNumeric ? parsed : (presetAppId ? String(presetAppId) : '');
  const platform = document.getElementById('versions-platform')?.value || 'iphone';

  const loading = document.getElementById('versions-loading');
  const container = document.getElementById('versions-container');
  const tableBody = document.getElementById('versions-table-body');
  const titleEl = document.getElementById('versions-app-title');
  const badgeEl = document.getElementById('versions-count-badge');
  const bundleEl = document.getElementById('versions-app-bundle');

  loading.style.display = 'block';
  container.style.display = 'none';

  try {
    const res = await fetch(`/api/versions?bundleId=${encodeURIComponent(bundleId)}&appId=${encodeURIComponent(appId)}&platform=${encodeURIComponent(platform)}`);
    const data = await res.json();
    loading.style.display = 'none';

    if (!data.success) {
      showToast(data.message || 'Не удалось получить версии приложения', 'error');
      return;
    }

    const versions = data.externalVersionIdentifiers || [];
    const appName = data.name || presetName || '';
    const resolvedAppId = Number(data.appID || appId) || 0;
    if (resolvedAppId > 0) {
      state.lastVersionsAppId = resolvedAppId;
      state.lastVersionsName = appName;
      state.lastVersionsInput = parsed;
    }
    titleEl.textContent = appName || data.bundleID || parsed;
    bundleEl.textContent = `Bundle ID: ${data.bundleID || parsed}`;
    badgeEl.textContent = `${versions.length} версий`;

    // Show versions in reverse order (newest first), one page at a time.
    state.versionsPage = {
      ids: [...versions].reverse(),
      page: 1,
      ctx: {
        bundleId: data.bundleID || bundleId,
        appId: resolvedAppId,
        appName: appName || data.bundleID || parsed,
        platform
      }
    };

    container.style.display = 'block';
    renderVersionsPage();
  } catch (err) {
    loading.style.display = 'none';
    showToast('Ошибка связи с сервером', 'error');
  }
}

// ==========================================
// Version list paging (shared by the Version History tab and the batch tab)
// ==========================================

// Number of builds shown per page. Apps like Chrome or Instagram have several
// hundred builds; rendering all of them at once (and fetching metadata for
// each) made the page sluggish and hammered Apple with requests.
const VERSIONS_PAGE_SIZE_KEY = 'ipatool_versions_page_size';
const VERSIONS_PAGE_SIZES = [25, 50, 100, 0]; // 0 = all

function versionsPageSize() {
  const stored = parseInt(localStorage.getItem(VERSIONS_PAGE_SIZE_KEY), 10);
  if (VERSIONS_PAGE_SIZES.includes(stored)) return stored;
  return 50;
}

function setVersionsPageSize(size) {
  localStorage.setItem(VERSIONS_PAGE_SIZE_KEY, String(size));
}

// Splits `ids` into the slice for `page` and returns paging info.
function versionsSlice(ids, page) {
  const size = versionsPageSize();
  const total = ids.length;
  const pages = size > 0 ? Math.max(1, Math.ceil(total / size)) : 1;
  const current = Math.min(Math.max(1, page), pages);
  const from = size > 0 ? (current - 1) * size : 0;
  const to = size > 0 ? Math.min(total, from + size) : total;
  return { size, total, pages, page: current, from, to, ids: ids.slice(from, to) };
}

// Builds the pager toolbar (first/prev/next/last, page-size select). `onPage`
// is invoked with the new page number; `onSize` after the page size changed.
function buildVersionsPager(info, onPage, onSize) {
  const pager = document.createElement('div');
  pager.className = 'versions-pager';

  const summary = document.createElement('span');
  summary.className = 'versions-pager-summary text-secondary';
  summary.textContent = info.total === 0
    ? ''
    : `${batchText('pager_range', { from: info.from + 1, to: info.to, total: info.total })} · ${batchText('pager_page_of', { page: info.page, pages: info.pages })}`;
  pager.appendChild(summary);

  const nav = document.createElement('div');
  nav.className = 'versions-pager-nav';

  const mkBtn = (label, target, disabled) => {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'btn btn-outline btn-sm';
    b.textContent = label;
    b.disabled = disabled;
    b.addEventListener('click', () => onPage(target));
    return b;
  };

  if (info.pages > 1) {
    nav.appendChild(mkBtn(batchText('pager_first'), 1, info.page === 1));
    nav.appendChild(mkBtn(batchText('pager_prev'), info.page - 1, info.page === 1));
    nav.appendChild(mkBtn(batchText('pager_next'), info.page + 1, info.page === info.pages));
    nav.appendChild(mkBtn(batchText('pager_last'), info.pages, info.page === info.pages));
  }

  const sizeLabel = document.createElement('label');
  sizeLabel.className = 'versions-pager-size text-secondary';
  sizeLabel.textContent = batchText('pager_per_page') + ' ';
  const select = document.createElement('select');
  select.className = 'select-control select-sm';
  VERSIONS_PAGE_SIZES.forEach(size => {
    const opt = document.createElement('option');
    opt.value = String(size);
    opt.textContent = size === 0 ? batchText('pager_all') : String(size);
    opt.selected = size === info.size;
    select.appendChild(opt);
  });
  select.addEventListener('change', () => {
    setVersionsPageSize(parseInt(select.value, 10));
    onSize();
  });
  sizeLabel.appendChild(select);
  nav.appendChild(sizeLabel);

  pager.appendChild(nav);
  return pager;
}

// Fetches display version / release date / minimum OS for the given builds.
// The visible page is sent to the server in chunks (POST /api/version-metadata/batch)
// and the server fans the Apple calls of each chunk out in parallel. Doing it
// client-side with one fetch per build never got past the browser's
// ~6-connections-per-host cap, while Apple answers each metadata request in
// ~25 s regardless of parallelism — so one chunk finishes in roughly the time
// of a single request. 50 per chunk triggered HTTP 502 for about a third of
// the uncached builds, hence 10 (a 50-row page = 5 sequential chunks). Must
// not exceed versionMetadataBatchLimit in cmd/gui.go.
// `fill(vId, data|null)` writes one result into the table; `isStale()` lets
// the caller abort when the user already moved to another page.
const METADATA_BATCH_SIZE = 5;
const METADATA_CONCURRENCY = 2;

async function fillVersionMetadata(ids, params, fill, isStale) {
  const chunks = [];
  for (let i = 0; i < ids.length; i += METADATA_BATCH_SIZE) {
    chunks.push(ids.slice(i, i + METADATA_BATCH_SIZE));
  }

  let nextIdx = 0;
  const worker = async () => {
    while (nextIdx < chunks.length) {
      if (isStale && isStale()) return;
      const chunk = chunks[nextIdx++];
      let results = {};
      try {
        const res = await fetch('/api/version-metadata/batch', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...params, versionIds: chunk }),
        });
        const data = await res.json();
        results = (data && data.results) || {};
      } catch (err) {
        results = {};
      }
      if (isStale && isStale()) return;
      chunk.forEach(vId => {
        const item = results[vId];
        fill(vId, item && item.success ? item : null);
      });
    }
  };

  const workers = [];
  const workerCount = Math.min(METADATA_CONCURRENCY, chunks.length);
  for (let w = 0; w < workerCount; w++) {
    workers.push(worker());
  }
  await Promise.all(workers);
}

// Renders the current page of the Version History tab.
function renderVersionsPage() {
  const ps = state.versionsPage;
  const tableBody = document.getElementById('versions-table-body');
  const pagerTop = document.getElementById('versions-pager-top');
  const pagerBottom = document.getElementById('versions-pager-bottom');
  if (!ps || !tableBody) return;

  const info = versionsSlice(ps.ids, ps.page);
  ps.page = info.page;
  const token = (ps.renderToken = (ps.renderToken || 0) + 1);
  const { bundleId, appId, appName, platform = 'iphone' } = ps.ctx;
  const appNameEsc = (appName || '').replace(/'/g, "\\'");

  tableBody.innerHTML = '';
  info.ids.forEach((vId, i) => {
    const globalIdx = info.from + i;
    const row = document.createElement('tr');
    row.innerHTML = `
      <td><code>${vId}</code> ${globalIdx === 0 ? `<span class="badge badge-success">${batchText('batch_latest_badge')}</span>` : ''}</td>
      <td id="disp-ver-${vId}">…</td>
      <td id="date-ver-${vId}">…</td>
      <td>
        <button class="btn btn-primary btn-sm" onclick="startAppDownload('${bundleId}', ${appId}, '${platform}', '${appNameEsc || bundleId}', '', '${vId}')">
          ⬇️ ${batchText('batch_download_single')}
        </button>
      </td>
    `;
    tableBody.appendChild(row);
  });

  const goPage = page => { ps.page = page; renderVersionsPage(); scrollVersionsTableIntoView(); };
  const onSize = () => { ps.page = 1; renderVersionsPage(); };
  [pagerTop, pagerBottom].forEach(el => {
    if (!el) return;
    el.innerHTML = '';
    el.appendChild(buildVersionsPager(info, goPage, onSize));
  });

  fillVersionMetadata(
    info.ids,
    { bundleId: bundleId || '', appId: Number(appId) || 0, platform },
    (vId, data) => {
      const dispEl = document.getElementById(`disp-ver-${vId}`);
      const dateEl = document.getElementById(`date-ver-${vId}`);
      if (dispEl) dispEl.innerHTML = data ? renderDisplayVersionCell(data.displayVersion, data.minimumOSVersion, platform) : '—';
      if (dateEl) dateEl.textContent = data ? (data.releaseDate || '—') : '—';
    },
    () => ps.renderToken !== token
  );
}

function scrollVersionsTableIntoView() {
  const el = document.getElementById('versions-pager-top');
  if (el && typeof el.scrollIntoView === 'function') el.scrollIntoView({ block: 'start', behavior: 'smooth' });
}

// ==========================================
// Purchase History (owned apps)
// ==========================================

// Called whenever the authenticated account may have changed (startup status
// startup check, login or logout). Loads the purchase list once per
// account; explicit refreshes go through refreshPurchases(true).
function onAuthStateChanged() {
  const p = state.purchases;
  const dsid = state.isAuthenticated && state.account ? (state.account.dsid || state.account.email) : null;

  if (!dsid) {
    p.apps = [];
    p.loaded = false;
    p.loading = false;
    p.error = '';
    p.fetchedAt = null;
    p.fromCache = false;
    p.loadedFor = null;
    renderPurchases();
    return;
  }

  if (p.loadedFor !== dsid) {
    p.apps = [];
    p.loaded = false;
    p.error = '';
    p.fetchedAt = null;
    p.fromCache = false;
    p.loadedFor = dsid;
    refreshPurchases(false);
  }
}

// Loads the purchase history. force=true bypasses the server-side cache and
// asks Apple again; force=false returns the cached copy when one exists.
async function refreshPurchases(force) {
  const p = state.purchases;
  if (!state.isAuthenticated) {
    renderPurchases();
    return;
  }
  if (p.loading) return;

  p.loading = true;
  p.error = '';
  renderPurchases();

  try {
    const res = await fetch(`/api/purchases${force ? '?refresh=1' : ''}`);
    const data = await res.json().catch(() => ({}));

    if (!res.ok || !data.success) {
      p.error = data.message || `HTTP ${res.status}`;
      if (res.status === 401 && !force) {
        // Session is unusable; do not spam toasts on the silent initial load.
        console.warn('purchase history:', p.error);
      } else {
        showToast(`${batchText('purchases_load_failed')}: ${p.error}`, 'error');
      }
      return;
    }

    // Current GUI servers return `items` with a Unix-seconds timestamp;
    // accept the older `apps`/ISO timestamp shape as well.
    const rawItems = Array.isArray(data.items) ? data.items : (Array.isArray(data.apps) ? data.apps : []);
    p.apps = rawItems.map(normalizePurchaseItem);
    p.loaded = true;
    p.fetchedAt = parsePurchaseDate(data.fetchedAt) || new Date();
    p.fromCache = !!data.cached;

    if (force) {
      showToast(batchText('purchases_loaded_toast', { count: p.apps.length }), 'success');
    }
  } catch (err) {
    p.error = err.message || String(err);
    if (force) showToast(batchText('purchases_load_failed'), 'error');
  } finally {
    p.loading = false;
    renderPurchases();
  }
}

function normalizePurchaseItem(item) {
  if (!item || typeof item !== 'object') return {};
  return {
    ...item,
    appId: item.appId ?? item.trackId ?? item.id ?? 0,
    bundleId: item.bundleId ?? item.bundleID ?? item.bundleIdentifier ?? '',
    name: item.name ?? item.trackName ?? '',
    version: item.version ?? item.displayVersion ?? '',
    tvosVersion: item.tvosVersion ?? '',
    purchaseDate: item.purchaseDate ?? item.date ?? '',
    hiddenFromStorefront: !!(item.hiddenFromStorefront || item.delisted)
  };
}

// Go sends fetchedAt as Unix seconds and purchaseDate as `YYYY-MM-DD HH:mm:ss`;
// normalize both, while continuing to accept ISO strings from older/demo APIs.
function parsePurchaseDate(value) {
  if (value instanceof Date) return Number.isNaN(value.getTime()) ? null : value;
  if (value === null || value === undefined || value === '') return null;

  if (typeof value === 'number' || (typeof value === 'string' && /^\d+(?:\.\d+)?$/.test(value.trim()))) {
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return null;
    const milliseconds = Math.abs(numeric) < 1e12 ? numeric * 1000 : numeric;
    const date = new Date(milliseconds);
    return Number.isNaN(date.getTime()) ? null : date;
  }

  let text = String(value).trim();
  if (!text) return null;
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(text)) {
    text = text.replace(' ', 'T');
  }
  const date = new Date(text);
  return Number.isNaN(date.getTime()) ? null : date;
}

function formatPurchaseDate(value) {
  const d = parsePurchaseDate(value);
  if (!d) return '—';
  return d.toLocaleString(state.lang === 'ru' ? 'ru-RU' : 'en-US', {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit'
  });
}

function purchaseAppOSLabels(a) {
  const validPlatforms = Array.isArray(a?.platforms)
    ? a.platforms.filter(pl => pl && String(pl).toLowerCase() !== 'unknown')
    : [];
  return [...new Set((validPlatforms.length ? validPlatforms : ['iphone']).map(pl => platformOSLabel(pl)))];
}

function filteredPurchases() {
  const p = state.purchases;
  const q = (document.getElementById('purchases-filter')?.value || '').trim().toLowerCase();
  const osFilter = document.getElementById('purchases-os-filter')?.value || 'all';
  const onlyHidden = !!document.getElementById('purchases-only-hidden')?.checked;
  return p.apps.filter(a => {
    if (onlyHidden && !a.hiddenFromStorefront) return false;
    if (osFilter && osFilter !== 'all' && !purchaseAppOSLabels(a).includes(osFilter)) return false;
    if (!q) return true;
    return (
      (a.name || '').toLowerCase().includes(q) ||
      (a.bundleId || '').toLowerCase().includes(q) ||
      String(a.appId || '').includes(q)
    );
  });
}

function renderPurchases() {
  const p = state.purchases;
  const els = {
    needAuth: document.getElementById('purchases-need-auth'),
    loading: document.getElementById('purchases-loading'),
    error: document.getElementById('purchases-error'),
    errorText: document.getElementById('purchases-error-text'),
    empty: document.getElementById('purchases-empty'),
    container: document.getElementById('purchases-container'),
    body: document.getElementById('purchases-table-body'),
    countBadge: document.getElementById('purchases-count-badge'),
    navBadge: document.getElementById('purchases-badge'),
    updated: document.getElementById('purchases-updated'),
    refreshBtn: document.getElementById('purchases-refresh-btn')
  };
  if (!els.container) return; // tab markup not present

  const show = (el, on) => { if (el) el.style.display = on ? '' : 'none'; };

  // Nav badge with total owned count.
  if (els.navBadge) {
    if (p.loaded && p.apps.length > 0) {
      els.navBadge.textContent = p.apps.length;
      els.navBadge.style.display = '';
    } else {
      els.navBadge.style.display = 'none';
    }
  }

  if (els.refreshBtn) {
    els.refreshBtn.classList.toggle('is-loading', p.loading);
    els.refreshBtn.disabled = !state.isAuthenticated;
  }

  if (els.updated) {
    if (p.fetchedAt) {
      let txt = batchText('purchases_updated_at', { time: formatPurchaseDate(p.fetchedAt.toISOString()) });
      if (p.fromCache) txt += ` (${batchText('purchases_from_cache')})`;
      els.updated.textContent = txt;
      const sevenDaysMs = 7 * 24 * 60 * 60 * 1000;
      const isStale = (Date.now() - p.fetchedAt.getTime()) > sevenDaysMs;
      els.updated.classList.toggle('is-stale', isStale);
      els.updated.title = isStale
        ? (state.lang === 'en'
            ? 'Purchase history cache has not been updated for more than 7 days'
            : 'Кэш истории покупок не обновлялся более 7 дней')
        : '';
    } else {
      els.updated.textContent = '';
      els.updated.classList.remove('is-stale');
      els.updated.title = '';
    }
  }

  show(els.needAuth, false);
  show(els.loading, false);
  show(els.error, false);
  show(els.empty, false);
  show(els.container, false);

  if (!state.isAuthenticated) {
    show(els.needAuth, true);
    if (els.countBadge) els.countBadge.textContent = '0';
    return;
  }

  if (p.loading && !p.loaded) {
    show(els.loading, true);
    return;
  }

  if (p.error && !p.loaded) {
    if (els.errorText) els.errorText.textContent = p.error;
    show(els.error, true);
    return;
  }

  if (p.loaded && p.apps.length === 0) {
    show(els.empty, true);
    if (els.countBadge) els.countBadge.textContent = '0';
    return;
  }

  const list = filteredPurchases();
  const osFilter = document.getElementById('purchases-os-filter')?.value || 'all';
  const hiddenTotal = p.apps.filter(a => {
    if (!a.hiddenFromStorefront) return false;
    if (osFilter && osFilter !== 'all' && !purchaseAppOSLabels(a).includes(osFilter)) return false;
    return true;
  }).length;
  const hiddenCountEl = document.getElementById('purchases-hidden-count');
  if (hiddenCountEl) {
    hiddenCountEl.textContent = String(hiddenTotal);
  }
  const hiddenWrapEl = document.getElementById('purchases-hidden-toggle-wrap');
  if (hiddenWrapEl) {
    hiddenWrapEl.title = state.lang === 'en'
      ? 'Show only apps hidden or removed from the regional App Store storefront'
      : 'Показать только приложения, скрытые или удалённые из региональной витрины App Store';
  }
  if (els.countBadge) {
    els.countBadge.textContent = list.length === p.apps.length
      ? `${p.apps.length}`
      : `${list.length} / ${p.apps.length}`;
  }

  show(els.container, true);
  if (!els.body) return;

  if (list.length === 0) {
    els.body.innerHTML = `<tr><td colspan="5" class="text-secondary" style="text-align:center;padding:24px">${batchEscapeHtml(batchText('purchases_no_match'))}</td></tr>`;
    return;
  }

  const dlLabel = batchEscapeHtml(batchText('purchases_download'));
  const verLabel = batchEscapeHtml(batchText('purchases_versions'));
  const unknown = batchText('purchases_unknown_app');
  const hiddenTip = batchEscapeHtml(
    state.lang === 'en'
      ? 'Hidden or removed from the regional App Store storefront'
      : 'Скрыто или удалено из региональной витрины App Store'
  );

  const rows = list.map(a => {
    const name = a.name || unknown;
    const nameEsc = batchEscapeHtml(name);
    const bundleEsc = batchEscapeHtml(a.bundleId || '');
    const nameJs = name.replace(/\\/g, '\\\\').replace(/'/g, "\\'");
    const bundleJs = (a.bundleId || '').replace(/\\/g, '\\\\').replace(/'/g, "\\'");
    const appId = Number(a.appId) || 0;
    const validPlatforms = Array.isArray(a.platforms)
      ? a.platforms.filter(pl => pl && String(pl).toLowerCase() !== 'unknown')
      : [];
    const matchedPlatform = (osFilter && osFilter !== 'all')
      ? validPlatforms.find(pl => platformOSLabel(pl) === osFilter)
      : '';
    const itemPlatform = matchedPlatform || validPlatforms[0] || 'iphone';
    const osLabels = purchaseAppOSLabels(a);
    const displayVer = (osFilter === 'tvOS' || (osLabels.length === 1 && osLabels[0] === 'tvOS'))
      ? (a.tvosVersion || a.version)
      : (a.version || a.tvosVersion);
    const osBadgesCell = osLabels
      .map(lbl => {
        if (lbl === 'tvOS' && osFilter !== 'tvOS' && a.tvosVersion && a.version && a.tvosVersion !== a.version) {
          return ` <span class="badge badge-ios">${batchEscapeHtml(`tvOS ${a.tvosVersion}`)}</span>`;
        }
        return ` <span class="badge badge-ios">${batchEscapeHtml(lbl)}</span>`;
      })
      .join('');
    const versionCell = displayVer ? ` <span class="badge badge-version">${batchEscapeHtml(displayVer)}</span>` : '';
    const hiddenCell = a.hiddenFromStorefront ? `<span class="badge-hidden-storefront" title="${hiddenTip}">!</span>` : '';
    return `
      <tr>
        <td>${nameEsc}${hiddenCell}${versionCell}${osBadgesCell}</td>
        <td><code>${bundleEsc || '—'}</code></td>
        <td><code>${appId || '—'}</code></td>
        <td>${formatPurchaseDate(a.purchaseDate)}</td>
        <td>
          <div class="purchases-row-actions">
            <button class="btn btn-primary btn-sm" onclick="startAppDownload('${bundleJs}', ${appId}, '${itemPlatform}', '${nameJs}', '')">⬇️ ${dlLabel}</button>
            <button class="btn btn-outline btn-sm" onclick="viewAppVersions('${bundleJs}', ${appId}, '${nameJs}', '${itemPlatform}')">🕒 ${verLabel}</button>
          </div>
        </td>
      </tr>`;
  });

  els.body.innerHTML = rows.join('');
}

// Saves the (filtered) purchase list in the same "Name: AppID" format the
// mass-download tab accepts, directly to the current working folder.
async function exportPurchasesList() {
  const list = filteredPurchases();
  if (!list.length) {
    showToast(batchText('purchases_nothing_to_export'), 'error');
    return;
  }

  const content = list
    .map(a => `${(a.name || a.bundleId || a.appId)}: ${a.appId}`)
    .join('\n') + '\n';

  const who = state.account ? (state.account.email || 'apple').replace(/[^\w.@-]+/g, '_') : 'apple';
  const filename = `purchases-${who}.txt`;

  try {
    const res = await fetch('/api/save-list', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename, content })
    });
    const data = await res.json();
    if (data && data.success) {
      showToast(batchText('purchases_saved_toast', { path: data.path || filename }), 'success');
      return;
    }
    showToast((data && data.message) || batchText('purchases_save_failed'), 'error');
  } catch (err) {
    showToast(batchText('purchases_save_failed'), 'error');
  }
}

// ==========================================
// Downloads & History Manager
// ==========================================

function updateDownloadsBadge() {
  const badge = document.getElementById('downloads-badge');
  const count = state.activeDownloads.size;
  if (badge) {
    if (count > 0) {
      badge.textContent = count;
      badge.style.display = 'inline-block';
    } else {
      badge.style.display = 'none';
    }
  }
}

// Renders the "Активные загрузки" section on the Downloads tab from the
// client-side job map. Terminal jobs are removed from the map by their own
// pollers, so this list only shows jobs that are still running.
function renderActiveDownloadsTab() {
  const listEl = document.getElementById('active-downloads-list');
  const noEl = document.getElementById('no-active-downloads');
  if (!listEl) return;

  const jobs = Array.from(state.activeDownloads.values())
    .sort((a, b) => (b.createdAt || 0) - (a.createdAt || 0));
  const active = jobs.filter(j => j.status !== 'completed' && j.status !== 'error');

  // If nothing is active, restore the built-in empty state placeholder.
  if (active.length === 0) {
    listEl.innerHTML = '';
    if (noEl) listEl.appendChild(noEl);
    return;
  }

  listEl.innerHTML = '';
  active.forEach(job => {
    const pct = Math.min(100, Math.max(0, job.progress || 0));
    const statusLabel = (i18n[state.lang] || i18n.ru)[`batch_download_status_${job.status}`] || job.status;
    const el = document.createElement('div');
    el.className = 'active-download-card';
    el.innerHTML = `
      <div class="active-dl-header">
        <div class="active-dl-title" title="${(job.appName || job.bundleId || job.id || '').replace(/"/g, '&quot;')}">
          ${(job.appName || job.bundleId || job.id || '').replace(/</g, '&lt;')}
        </div>
        <span class="badge ${job.status === 'error' ? 'badge-error' : 'badge-version'}">${statusLabel}</span>
      </div>
      <div class="dl-progress-bar-bg">
        <div class="dl-progress-bar-fill" style="width:${pct}%"></div>
      </div>
      <div class="dl-progress-stats">
        <span>${pct.toFixed(1)}%</span>
        <span>${job.totalBytes > 0 ? `${formatBytes(job.bytesRead)} / ${formatBytes(job.totalBytes)}` : (job.bytesRead ? formatBytes(job.bytesRead) : '')} ${job.speed || ''}</span>
      </div>
      ${job.status === 'error' ? `<div class="text-secondary" style="margin-top:6px">${(job.error || '').replace(/</g, '&lt;')}</div>` : ''}
    `;
    listEl.appendChild(el);
  });
}

// Fetches active jobs from the server so the Downloads tab is also correct
// after a page reload or for jobs started from other tabs (e.g. batch).
async function refreshActiveDownloads() {
  try {
    const res = await fetch('/api/downloads/active');
    const data = await res.json();
    if (!data.success) return;

    const live = new Map();
    (data.jobs || []).forEach(job => {
      if (job && job.id) live.set(job.id, job);
    });

    // Merge server state; keep locally-known jobs that are not yet reported
    // (e.g. just queued) and drop nothing while they are still active.
    live.forEach((job, id) => state.activeDownloads.set(id, job));
    const activeIds = new Set(live.keys());
    state.activeDownloads.forEach((job, id) => {
      // Once a job is terminal, its poller removes it; if the server no longer
      // lists it and it is not terminal here, drop the stale entry.
      if (!activeIds.has(id) && job.status !== 'completed' && job.status !== 'error') {
        state.activeDownloads.delete(id);
      }
    });

    updateDownloadsBadge();
    renderActiveDownloadsTab();

    // Stop background /api/downloads/active polling once no server-side jobs
    // remain or when trackDownloadProgress is already polling the single job.
    if (live.size === 0 || (state.isDownloading && live.size <= 1)) {
      stopActiveDownloadsPolling();
    }
  } catch (err) {
    console.error('Active downloads refresh error:', err);
  }
}

let activeDownloadsTimer = null;

function startActiveDownloadsPolling() {
  if (activeDownloadsTimer) return;
  if (state.isDownloading && state.activeDownloads.size <= 1) {
    renderActiveDownloadsTab();
    return;
  }
  refreshActiveDownloads();
  activeDownloadsTimer = setInterval(refreshActiveDownloads, 3000);
}

function stopActiveDownloadsPolling() {
  if (activeDownloadsTimer) {
    clearInterval(activeDownloadsTimer);
    activeDownloadsTimer = null;
  }
}

function addToHistory(item) {
  // Check for duplicates by outputPath (if provided) within last 10 seconds
  const now = Date.now();
  const isDuplicate = state.downloadHistory.some(existing => {
    // Match by outputPath if both have it
    if (item.outputPath && existing.outputPath && item.outputPath === existing.outputPath) {
      // Check if existing entry is recent (within 10 seconds)
      if (existing._timestamp) {
        const age = now - existing._timestamp;
        if (age < 10000) return true;
      }
    }
    return false;
  });
  
  if (isDuplicate) {
    return; // Skip duplicate
  }
  
  // Add timestamp for duplicate detection
  item._timestamp = now;
  
  state.downloadHistory.unshift(item);
  if (state.downloadHistory.length > 50) {
    state.downloadHistory = state.downloadHistory.slice(0, 50);
  }
  localStorage.setItem('ipatool_download_history', JSON.stringify(state.downloadHistory));
  renderDownloadsTab();
}

function renderDownloadsTab() {
  const historyList = document.getElementById('downloads-history-list');
  const noHistory = document.getElementById('no-history-downloads');
  if (!historyList) return;

  if (state.downloadHistory.length === 0) {
    historyList.innerHTML = '';
    if (noHistory) historyList.appendChild(noHistory);
    return;
  }

  historyList.innerHTML = '';
  state.downloadHistory.forEach((item, idx) => {
    const el = document.createElement('div');
    el.className = 'history-item';
    const fileName = item.outputPath ? item.outputPath.split(/[\\/]/).pop() : `${item.bundleId}.ipa`;

    el.innerHTML = `
      <div class="history-info">
        <div class="history-file-icon">📦</div>
        <div>
          <div class="history-filename">${item.appName || fileName}</div>
          <div class="history-meta">${fileName} • ${formatBytes(item.bytes)} • ${item.date || ''}</div>
        </div>
      </div>
      <div class="app-actions">
        <button class="btn btn-outline btn-sm" onclick="openOutputFolder('${item.outputPath ? item.outputPath.replace(/\\/g, '\\\\') : ''}')">
          📂 В проводнике
        </button>
        <button class="btn btn-outline btn-sm" onclick="copyToClipboard('${item.outputPath ? item.outputPath.replace(/\\/g, '\\\\') : ''}')">
          📋 Путь
        </button>
        <button class="icon-btn-inline" onclick="removeFromHistory(${idx})" title="Удалить из списка">
          🗑️
        </button>
      </div>
    `;
    historyList.appendChild(el);
  });
}

function removeFromHistory(idx) {
  state.downloadHistory.splice(idx, 1);
  localStorage.setItem('ipatool_download_history', JSON.stringify(state.downloadHistory));
  renderDownloadsTab();
}

function refreshDownloadsHistory() {
  renderDownloadsTab();
  showToast('Список обновлен', 'info');
}

async function openOutputFolder(path = '') {
  try {
    const res = await fetch('/api/open-folder', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path })
    });
    const data = await res.json();
    if (!data.success) {
      showToast(data.message || 'Не удалось открыть папку', 'error');
    }
  } catch (err) {
    showToast('Ошибка при открытии папки', 'error');
  }
}

// ==========================================
// Batch Mass Download Tab
// ==========================================

const batchState = {
  items: [],              // parsed [{ appId, name }]
  results: null,          // last check job payload
  selected: new Set(),    // appIds whose card checkbox is ticked (download latest)
  selectedVersions: {},   // appId -> a specific versionId picked in version history
  addedToHistory: new Set(),
  checkPoll: null,
  downloadPoll: null,
  batchId: null,        // id of the running mass-download job (needed to skip)
  retryable: [],        // items left over when a batch finished (retry queue)
  retrying: false
};

// Watchdog settings are read straight from the form so a retry reuses
// whatever the user currently has configured.
function batchWatchdogSettings() {
  return {
    stallTimeoutSec: parseInt(document.getElementById('batch-stall-timeout')?.value, 10) || 0,
    itemTimeoutSec: parseInt(document.getElementById('batch-item-timeout')?.value, 10) || 0
  };
}

// An item the batch gave up on: either the watchdog fired or the user skipped
// it. Errors are included too so the retry queue covers every failure.
function isBatchRetryable(item) {
  return item.status === 'stalled' || item.status === 'skipped' || item.status === 'error';
}

// Parse a txt list into [{appId, name}]. Supports "Name: 568903335",
// plain IDs, App Store URLs and various separators.
function parseBatchList(text) {
  const items = [];
  const seen = new Set();

  (text || '').split(/\r?\n/).forEach(raw => {
    const line = raw.trim();
    if (!line) return;

    const urlMatch = line.match(/id(\d{4,})/i);
    let idStr = urlMatch ? urlMatch[1] : null;
    let namePart = line;

    if (idStr) {
      namePart = line.replace(new RegExp('id' + idStr + '\\b', 'i'), '').trim();
      if (/^https?:\/\/|apps\.apple\.com|\//i.test(namePart)) {
        namePart = '';
      }
    } else {
      const nums = line.match(/\d{4,}/g);
      if (!nums || nums.length === 0) return;
      idStr = nums[nums.length - 1];
      namePart = line.slice(0, line.lastIndexOf(idStr)).trim();
    }

    const appId = parseInt(idStr, 10);
    if (!appId || seen.has(appId)) return;
    seen.add(appId);

    namePart = namePart.replace(/^[\s\-–—:;,.()]+|[\s\-–—:;,.()]+$/g, '').trim();
    items.push({ appId, name: namePart || String(appId) });
  });

  return items;
}

function batchText(key, vars) {
  let text = (i18n[state.lang] || i18n.ru)[key] || key;
  if (vars) {
    Object.keys(vars).forEach(k => {
      text = text.replaceAll(`{${k}}`, vars[k]);
    });
  }
  return text;
}

function batchEscapeHtml(str) {
  return String(str || '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function updateBatchParsedSummary() {
  const summary = document.getElementById('batch-parsed-summary');
  const countEl = document.getElementById('batch-parsed-count');
  const input = document.getElementById('batch-paste-area');
  const items = parseBatchList(input ? input.value : '');
  if (summary) summary.style.display = items.length > 0 ? 'block' : 'none';
  if (countEl) countEl.textContent = batchText('batch_parsed_count', { count: items.length });
}

async function handleBatchFileChange(e) {
  const file = e.target.files && e.target.files[0];
  if (!file) return;
  const text = await file.text();
  const pasteArea = document.getElementById('batch-paste-area');
  if (pasteArea) pasteArea.value = text;
  updateBatchParsedSummary();
}

async function handleBatchCheck(e) {
  e.preventDefault();

  if (!state.isAuthenticated) {
    showToast(batchText('batch_need_auth'), 'error');
    switchTab('account');
    return;
  }

  const pasteArea = document.getElementById('batch-paste-area');
  const items = parseBatchList(pasteArea ? pasteArea.value : '');
  if (items.length === 0) {
    showToast(batchText('batch_no_items'), 'error');
    return;
  }

  const platform = document.getElementById('batch-platform')?.value || 'iphone';
  const purchase = document.getElementById('batch-purchase')?.checked !== false;
  const outputEl = document.getElementById('batch-parsed-summary');
  const btn = document.getElementById('batch-check-btn');
  if (btn) btn.disabled = true;

  try {
    const res = await fetch('/api/batch/check', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        platform,
        items,
        purchase,
        // A probe that never answers would hold up the rest of the list, so
        // it is time-boxed too.
        itemTimeoutSec: parseInt(document.getElementById('batch-check-timeout')?.value, 10) || 0
      })
    });
    const data = await res.json();
    if (!data.success) {
      showToast(data.message || 'Ошибка запуска проверки', 'error');
      return;
    }
    batchState.items = items;
    batchState.results = null;
    batchState.selected = new Set();
    batchState.addedToHistory = new Set();
    document.getElementById('batch-results-card').style.display = 'none';
    document.getElementById('batch-download-card').style.display = 'none';
    document.getElementById('batch-check-progress-card').style.display = 'block';
    document.getElementById('batch-check-progress-fill').style.width = '0%';
    document.getElementById('batch-check-progress-percent').textContent = '0%';
    pollBatchCheck(data.jobId, items);
  } catch (err) {
    showToast('Ошибка связи с сервером', 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

function pollBatchCheck(jobId, items) {
  if (batchState.checkPoll) clearInterval(batchState.checkPoll);
  const fillEl = document.getElementById('batch-check-progress-fill');
  const percentEl = document.getElementById('batch-check-progress-percent');
  const textEl = document.getElementById('batch-check-progress-text');

  // Pre-build the status list so each line can be updated in place.
  const statusBox = document.getElementById('batch-available-list');
  if (statusBox) {
    statusBox.innerHTML = '';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'batch-status-row';
      row.id = `batch-row-${item.appId}`;
      row.innerHTML = `
        <span class="batch-status-icon" id="batch-status-icon-${item.appId}">⏳</span>
        <span class="batch-status-name" title="${batchEscapeHtml(item.name)}">${batchEscapeHtml(item.name)}</span>
        <code class="batch-status-id">${item.appId}</code>
        <span class="batch-status-label" id="batch-status-label-${item.appId}">…</span>
      `;
      statusBox.appendChild(row);
    });
  }

  batchState.checkPoll = setInterval(async () => {
    try {
      const res = await fetch(`/api/batch/check/status?jobId=${jobId}`);
      const job = await res.json();
      if (!job.id) {
        clearInterval(batchState.checkPoll);
        return;
      }

      const pct = Math.min(100, Math.max(0, job.progress || 0));
      if (fillEl) fillEl.style.width = `${pct}%`;
      if (percentEl) percentEl.textContent = `${pct.toFixed(1)}%`;
      if (textEl) textEl.textContent = batchText('batch_check_progress_text', {
        done: job.done || 0,
        total: job.total || 0,
        pct: job.total > 0 ? (100 / job.total).toFixed(2) : '0'
      });

      (job.items || []).forEach(item => {
        const iconEl = document.getElementById(`batch-status-icon-${item.appId}`);
        const labelEl = document.getElementById(`batch-status-label-${item.appId}`);
        if (iconEl) {
          iconEl.textContent = item.status === 'available' ? '✅' :
            item.status === 'license-required' ? '⛔' :
            item.status === 'stalled' ? '⏱️' :
            item.status === 'error' ? '⚠️' : '⏳';
        }
        if (labelEl) {
          labelEl.textContent = item.status === 'available' ? `v${item.version || '?'}` :
            item.status === 'license-required' ? 'license is required' :
            item.status === 'stalled' ? batchText('batch_stalled_status') :
            item.status === 'error' ? (item.error || 'Ошибка') : '…';
        }
      });

      if (job.status === 'completed') {
        clearInterval(batchState.checkPoll);
        batchState.results = job;
        renderBatchResults(job);
      }
    } catch (err) {
      console.error('Batch check poll error:', err);
    }
  }, 500);
}

function renderBatchResults(job) {
  const resultsCard = document.getElementById('batch-results-card');
  const summaryEl = document.getElementById('batch-results-summary');
  const availableEl = document.getElementById('batch-available-list');
  const filteredBox = document.getElementById('batch-filtered-box');
  const filteredEl = document.getElementById('batch-filtered-list');
  const platform = document.getElementById('batch-platform')?.value || 'iphone';
  const outputPath = document.getElementById('batch-output-path')?.value.trim() || '';

  const available = (job.items || []).filter(i => i.status === 'available');
  const filtered = (job.items || []).filter(i => i.status !== 'available');

  if (summaryEl) {
    summaryEl.textContent = batchText('batch_results_summary', {
      available: available.length,
      total: job.total || 0,
      filtered: filtered.length
    });
  }

  if (availableEl) {
    availableEl.innerHTML = '';
    available.forEach((item, idx) => {
      const card = document.createElement('div');
      card.className = 'app-card batch-app-card';
      card.innerHTML = `
        <div class="batch-app-select">
          <label class="checkbox-label">
            <input type="checkbox" class="batch-app-checkbox" data-appid="${item.appId}" ${idx < 10 ? 'checked' : ''}>
            <span class="checkbox-custom"></span>
            <span class="checkbox-text">
              <span class="app-name">${batchEscapeHtml(item.name)}</span>
              <span class="app-artist">App ID: <code>${item.appId}</code> · v${batchEscapeHtml(item.version || '?')}</span>
            </span>
          </label>
        </div>
        <div class="app-actions">
          <details class="batch-versions-details" data-appid="${item.appId}" data-name="${batchEscapeHtml(item.name)}">
            <summary class="btn btn-outline btn-sm">🕒 ${batchText('batch_version_history')}</summary>
            <div class="batch-versions-box" id="batch-versions-${item.appId}">
              <div class="text-secondary">${batchText('batch_versions_loading')}</div>
            </div>
          </details>
        </div>
      `;
      card.querySelector('.batch-app-checkbox').addEventListener('change', ev => {
        if (ev.target.checked) {
          batchState.selected.add(item.appId);
        } else {
          batchState.selected.delete(item.appId);
        }
        // Toggling the card checkbox controls the "latest version" selection;
        // it clears any specific version picked in the history table and
        // unticks its checkbox so the two selections never conflict.
        if (batchState.selectedVersions[item.appId]) {
          delete batchState.selectedVersions[item.appId];
          document.querySelectorAll(`.batch-version-checkbox[data-appid="${item.appId}"]`).forEach(box => {
            box.checked = false;
          });
        }
      });
      if (idx < 10) batchState.selected.add(item.appId);

      const details = card.querySelector('.batch-versions-details');
      details.addEventListener('toggle', () => {
        if (details.open) loadBatchVersions(item, platform, outputPath);
      });

      availableEl.appendChild(card);
    });
    if (available.length === 0) {
      availableEl.innerHTML = `<div class="state-container card"><div class="empty-icon">😕</div><p>${batchText('batch_no_items')}</p></div>`;
    }
  }

  if (filteredBox) filteredBox.style.display = filtered.length > 0 ? 'block' : 'none';
  if (filteredEl) {
    filteredEl.innerHTML = '';
    filtered.forEach(item => {
      const row = document.createElement('div');
      row.className = 'batch-filtered-row';
      row.innerHTML = `
        <span class="batch-status-icon">${item.status === 'license-required' ? '⛔' : item.status === 'stalled' ? '⏱️' : '⚠️'}</span>
        <span>${batchEscapeHtml(item.name)}</span>
        <code class="batch-status-id">${item.appId}</code>
        ${item.status === 'license-required'
          ? `<span class="badge badge-error">license is required</span>`
          : `<span class="badge badge-error">${batchEscapeHtml(item.error || 'Ошибка')}</span>`}
      `;
      filteredEl.appendChild(row);
    });
  }

  resultsCard.style.display = 'block';
}

// Loads version history for one app card (lazily on first expand). Uses
// app-scoped DOM ids so multiple apps can be expanded simultaneously. Unlike
// the standalone Version History tab (which has a per-row "Download" button),
// the batch variant shows a checkbox per version plus a "Download selected"
// button so several builds can be queued at once.
async function loadBatchVersions(item, platform, outputPath) {
  const container = document.getElementById(`batch-versions-${item.appId}`);
  if (!container || container.dataset.loaded === '1') return;
  container.dataset.loaded = '1';

  let ids = item.externalVersionIdentifiers || [];
  if (ids.length === 0) {
    // Fall back to the version-history endpoint when the check response did
    // not carry the build list.
    try {
      const res = await fetch(`/api/versions?appId=${item.appId}&platform=${encodeURIComponent(platform || 'iphone')}`);
      const data = await res.json();
      if (data.success && (data.externalVersionIdentifiers || []).length > 0) {
        ids = data.externalVersionIdentifiers;
      }
    } catch (err) { /* ignore */ }
  }

  if (ids.length === 0) {
    container.innerHTML = `<div class="text-secondary">${batchText('batch_no_versions')}</div>`;
    return;
  }

  const reversed = [...ids].reverse();
  const pageState = { page: 1, renderToken: 0, platform: platform || 'iphone' };

  // If a specific build was picked earlier, open the page that contains it.
  const picked = batchState.selectedVersions[item.appId];
  if (picked) {
    const idx = reversed.indexOf(picked);
    const size = versionsPageSize();
    if (idx >= 0 && size > 0) pageState.page = Math.floor(idx / size) + 1;
  }

  renderBatchVersionsPage(container, item, reversed, pageState);
}

// Renders one page of the batch-tab version table for a single app. The
// picked version is kept in batchState.selectedVersions, so switching pages
// does not lose the selection.
function renderBatchVersionsPage(container, item, reversed, pageState) {
  const info = versionsSlice(reversed, pageState.page);
  pageState.page = info.page;
  const token = ++pageState.renderToken;

  const table = document.createElement('table');
  table.className = 'versions-table batch-versions-table';
  table.innerHTML = `
    <thead>
      <tr>
        <th>${batchText('version_col_build')}</th>
        <th>${batchText('version_col_display')}</th>
        <th>${batchText('version_col_select')}</th>
      </tr>
    </thead>
    <tbody></tbody>
  `;
  const tbody = table.querySelector('tbody');

  info.ids.forEach((vId, i) => {
    const globalIdx = info.from + i;
    const row = document.createElement('tr');
    const isPicked = batchState.selectedVersions[item.appId] === vId;
    row.innerHTML = `
      <td><code>${vId}</code> ${globalIdx === 0 ? `<span class="badge badge-success">${batchText('batch_latest_badge')}</span>` : ''}</td>
      <td id="batch-disp-${item.appId}-${vId}">…</td>
      <td class="batch-version-select-cell">
        <label class="checkbox-label">
          <input type="checkbox" class="batch-version-checkbox" data-appid="${item.appId}" data-versionid="${batchEscapeHtml(vId)}" ${isPicked ? 'checked' : ''}>
          <span class="checkbox-custom"></span>
        </label>
      </td>
    `;
    const cb = row.querySelector('.batch-version-checkbox');
    cb.addEventListener('change', ev => {
      onBatchVersionPicked(item.appId, vId, ev.target.checked);
    });
    tbody.appendChild(row);
  });

  const hint = document.createElement('div');
  hint.className = 'batch-versions-hint text-secondary';
  hint.textContent = batchText('batch_version_pick_hint');

  const rerender = page => {
    pageState.page = page;
    renderBatchVersionsPage(container, item, reversed, pageState);
  };
  const pagerTop = buildVersionsPager(info, rerender, () => rerender(1));
  const pagerBottom = buildVersionsPager(info, rerender, () => rerender(1));

  container.innerHTML = '';
  container.appendChild(pagerTop);
  container.appendChild(table);
  if (info.pages > 1) container.appendChild(pagerBottom);
  container.appendChild(hint);

  fillVersionMetadata(
    info.ids,
    { appId: Number(item.appId) || 0, platform: pageState.platform || 'iphone' },
    (vId, data) => {
      // Only the display version is filled in: unlike the version history on
      // its own tab, the batch table has no release-date column.
      const dispEl = document.getElementById(`batch-disp-${item.appId}-${vId}`);
      if (dispEl) dispEl.innerHTML = data ? renderDisplayVersionCell(data.displayVersion, data.minimumOSVersion, pageState.platform) : '—';
    },
    () => pageState.renderToken !== token
  );
}

// Handles ticking/unticking a specific version in an app's version-history
// table. Picking a version means "download this build with the rest instead of
// the latest": it selects the app for the batch (and ticks the card checkbox),
// records the chosen versionId, and enforces a single pick per app. Unticking
// clears the pick so the app falls back to the latest version (card stays
// selected).
function onBatchVersionPicked(appId, versionId, checked) {
  if (checked) {
    // Only one version per app can be picked: untick any sibling checkbox.
    document.querySelectorAll(`.batch-version-checkbox[data-appid="${appId}"]`).forEach(box => {
      if (box.getAttribute('data-versionid') !== versionId) box.checked = false;
    });
    batchState.selectedVersions[appId] = versionId;
    batchState.selected.add(appId);
    // Ticking a version replaces the "latest" selection driven by the card
    // checkbox, so clear the card checkbox to signal a specific build is used.
    const cardBox = document.querySelector(`.batch-app-checkbox[data-appid="${appId}"]`);
    if (cardBox) cardBox.checked = false;
  } else {
    delete batchState.selectedVersions[appId];
    // No version picked anymore: fall back to the latest version and reflect
    // that by re-ticking the card checkbox.
    batchState.selected.add(appId);
    const cardBox = document.querySelector(`.batch-app-checkbox[data-appid="${appId}"]`);
    if (cardBox) cardBox.checked = true;
  }
}

// Saves the list of apps that passed the Apple ID check (status "available")
// back to a text file in the same "Name: AppID" format as Apps_ID_List.txt,
// so the remaining apps can be reused later.
async function saveBatchList() {
  const results = batchState.results;
  if (!results) return;

  const available = (results.items || []).filter(i => i.status === 'available');
  if (available.length === 0) {
    showToast(batchText('batch_no_items'), 'error');
    return;
  }

  const content = available
    .map(i => `${i.name || i.appId}: ${i.appId}`)
    .join('\n') + '\n';

  const filename = 'apps_list.txt';
  try {
    const res = await fetch('/api/save-list', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename, content })
    });
    const data = await res.json();
    if (data && data.success) {
      showToast(batchText('purchases_saved_toast', { path: data.path || filename }), 'success');
      return;
    }
    showToast((data && data.message) || batchText('purchases_save_failed'), 'error');
  } catch (err) {
    showToast(batchText('purchases_save_failed'), 'error');
  }
}

function batchSelectAll(select) {
  const boxes = document.querySelectorAll('.batch-app-checkbox');
  boxes.forEach(box => {
    box.checked = select;
    const appId = parseInt(box.getAttribute('data-appid'), 10);
    if (select) batchState.selected.add(appId);
    else batchState.selected.delete(appId);
  });
  // "Select all/none" drives the latest-version selection, so drop any specific
  // version picks and untick their checkboxes to keep the two views consistent.
  batchState.selectedVersions = {};
  document.querySelectorAll('.batch-version-checkbox').forEach(box => {
    box.checked = false;
  });
}

async function startBatchDownload() {
  if (!state.isAuthenticated) {
    showToast(batchText('batch_need_auth'), 'error');
    switchTab('account');
    return;
  }

  const results = batchState.results;
  if (!results) return;

  const available = (results.items || []).filter(i => i.status === 'available');
  const selected = available.filter(i => batchState.selected.has(i.appId));
  if (selected.length === 0) {
    showToast(batchText('batch_no_selected'), 'error');
    return;
  }

  await startBatchDownloadFor(selected, 'batch-download-btn');
}

// Shared entry point for a fresh mass download and for a retry of the items a
// previous run gave up on.
async function startBatchDownloadFor(items, btnId) {
  const platform = document.getElementById('batch-platform')?.value || 'iphone';
  const outputPath = document.getElementById('batch-output-path')?.value.trim() || '';
  const purchase = document.getElementById('batch-purchase')?.checked !== false;

  const btn = btnId ? document.getElementById(btnId) : null;
  if (btn) btn.disabled = true;

  try {
    const res = await fetch('/api/batch/download', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        platform,
        outputPath,
        purchase,
        ...batchWatchdogSettings(),
        items: items.map(i => ({
          appId: i.appId,
          name: i.name,
          externalVersionId: i.externalVersionId || i.versionId || batchState.selectedVersions[i.appId] || ''
        }))
      })
    });
    const data = await res.json();
    if (!data.success) {
      showToast(data.message || 'Ошибка запуска массовой загрузки', 'error');
      return false;
    }

    // A new batch replaces the previous rows, so drop the old retry queue.
    batchState.batchId = data.batchId;
    batchState.retryable = [];
    hideBatchRetry();

    const downloadCard = document.getElementById('batch-download-card');
    downloadCard.style.display = 'block';
    document.getElementById('batch-download-progress-fill').style.width = '0%';
    document.getElementById('batch-download-progress-percent').textContent = '0%';
    pollBatchDownload(data.batchId, items);
    // The progress card renders at the bottom of the page, so bring it into
    // view right away — otherwise it is not obvious the download has started.
    requestAnimationFrame(() => {
      downloadCard.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
    showToast(batchText('batch_download_started_toast'), 'success');

    return true;
  } catch (err) {
    showToast('Ошибка связи с сервером', 'error');
    return false;
  } finally {
    if (btn) btn.disabled = false;
  }
}

// Manual escape hatch: tell the server to give up on one app so the batch
// carries on with the next one. Useful when an app is visibly stuck but has
// not reached the automatic timeouts yet.
async function skipBatchItem(appId) {
  if (!batchState.batchId) return;

  try {
    const res = await fetch('/api/batch/download/skip', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ batchId: batchState.batchId, appId })
    });
    const data = await res.json();
    if (!data.success) {
      showToast(data.message || batchText('batch_skip_error'), 'error');
      return;
    }
    showToast(batchText('batch_skip_toast'), 'success');
  } catch (err) {
    showToast('Ошибка связи с сервером', 'error');
  }
}

function showBatchRetry(count) {
  const box = document.getElementById('batch-download-actions');
  if (!box) return;

  if (!count) {
    box.style.display = 'none';
    return;
  }

  const label = document.getElementById('batch-retry-count');
  if (label) label.textContent = ` (${count})`;
  box.style.display = 'block';
}

function hideBatchRetry() {
  const box = document.getElementById('batch-download-actions');
  if (box) box.style.display = 'none';
}

// Re-run only the apps the previous batch gave up on or failed on. Everything
// that already succeeded is left alone.
async function retryBatchFailed() {
  if (batchState.retrying || batchState.retryable.length === 0) return;

  batchState.retrying = true;

  try {
    await startBatchDownloadFor(batchState.retryable, 'batch-retry-btn');
  } finally {
    batchState.retrying = false;
  }
}

function pollBatchDownload(batchId, items) {
  if (batchState.downloadPoll) clearInterval(batchState.downloadPoll);
  const fillEl = document.getElementById('batch-download-progress-fill');
  const percentEl = document.getElementById('batch-download-progress-percent');
  const textEl = document.getElementById('batch-download-progress-text');
  const detailEl = document.getElementById('batch-download-progress-detail');
  const itemsEl = document.getElementById('batch-download-items');

  if (itemsEl) {
    itemsEl.innerHTML = '';
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'batch-download-row';
      row.id = `batch-dl-row-${item.appId}`;
      row.innerHTML = `
        <div class="batch-download-row-head">
          <span class="batch-status-name">${batchEscapeHtml(item.name)}</span>
          <code class="batch-status-id">${item.appId}</code>
          <span class="batch-status-label" id="batch-dl-label-${item.appId}">${batchText('batch_download_status_queued')}</span>
        </div>
        <div class="dl-progress-bar-bg slim">
          <div id="batch-dl-fill-${item.appId}" class="dl-progress-bar-fill" style="width:0%"></div>
        </div>
        <div class="batch-download-row-foot">
          <span id="batch-dl-detail-${item.appId}" class="text-secondary">—</span>
          <button id="batch-dl-skip-${item.appId}" class="btn btn-outline btn-sm" style="display:none" onclick="skipBatchItem(${item.appId})">⏭ ${batchText('batch_skip_btn')}</button>
          <button id="batch-dl-open-${item.appId}" class="btn btn-outline btn-sm" style="display:none" onclick="openOutputFolder('')">📂 ${batchText('open_folder_btn')}</button>
        </div>
      `;
      itemsEl.appendChild(row);
    });
  }

  batchState.downloadPoll = setInterval(async () => {
    try {
      const res = await fetch(`/api/batch/download/status?batchId=${batchId}`);
      const job = await res.json();
      if (!job.id) {
        clearInterval(batchState.downloadPoll);
        return;
      }

      const pct = Math.min(100, Math.max(0, job.progress || 0));
      if (fillEl) fillEl.style.width = `${pct}%`;
      if (percentEl) percentEl.textContent = `${pct.toFixed(1)}%`;
      if (textEl) {
        let summary = batchText('batch_download_progress_done', {
          done: job.completedCount || 0,
          total: job.total || 0,
          errors: job.errors || 0
        });
        if (job.skipped) {
          summary += ' ' + batchText('batch_download_skipped_count', { skipped: job.skipped });
        }
        textEl.textContent = summary;
      }

      (job.items || []).forEach(item => {
        const labelEl = document.getElementById(`batch-dl-label-${item.appId}`);
        const fill = document.getElementById(`batch-dl-fill-${item.appId}`);
        const detail = document.getElementById(`batch-dl-detail-${item.appId}`);
        const openBtn = document.getElementById(`batch-dl-open-${item.appId}`);
        const skipBtn = document.getElementById(`batch-dl-skip-${item.appId}`);

        // "Skip" is offered while the item is still doing something — it is
        // the way out when an app hangs below the automatic timeouts.
        if (skipBtn) {
          const busy = item.status === 'queued' || item.status === 'purchasing' ||
            item.status === 'downloading' || item.status === 'patching';
          skipBtn.style.display = busy ? 'inline-flex' : 'none';
        }

        const statusKey = `batch_download_status_${item.status}`;
        if (labelEl) labelEl.textContent = i18n[state.lang]?.[statusKey] || item.status;
        if (fill) fill.style.width = `${Math.min(100, item.progress || 0)}%`;
        if (detail) {
          let detailText;
          if (item.status === 'error' || item.status === 'stalled' || item.status === 'skipped') {
            detailText = item.error || '';
          } else if (item.totalBytes > 0) {
            const sizePart = `${formatBytes(item.bytesRead || 0)} / ${formatBytes(item.totalBytes)}`;
            detailText = item.outputPath ? `${sizePart} · ${item.outputPath}` : sizePart;
          } else {
            detailText = item.outputPath || '';
          }
          // Surface a hang early instead of waiting for the watchdog: while
          // an item is downloading, stalledFor counts the seconds that passed
          // without a single new byte.
          if (item.status === 'downloading' && (item.stalledFor || 0) >= 10) {
            detailText += (detailText ? ' · ' : '') +
              batchText('batch_stalled_hint', { sec: Math.round(item.stalledFor) });
          }
          detail.textContent = detailText;
          if (item.warning && !detail.querySelector('.batch-dl-warning')) {
            const warn = document.createElement('div');
            warn.className = 'batch-dl-warning';
            warn.textContent = '⚠️ ' + localizeJobWarning(item.warning);
            detail.appendChild(warn);
          }
        }

        if (item.status === 'completed') {
          if (openBtn) {
            openBtn.style.display = 'inline-flex';
            openBtn.setAttribute('onclick', `openOutputFolder('${(item.outputPath || '').replace(/\\/g, '\\\\').replace(/'/g, "\\'")}')`);
          }
          if (!batchState.addedToHistory.has(item.appId)) {
            batchState.addedToHistory.add(item.appId);
            addToHistory({
              appName: item.name,
              bundleId: '',
              version: '',
              outputPath: item.outputPath,
              bytes: item.totalBytes || item.bytesRead || 0,
              date: new Date().toLocaleString()
            });
          }
        }

        // Feed the Downloads tab / badge via the per-app job, and drop the
        // entry once its job reaches a terminal state.
        if (item.jobId) {
          if (item.status === 'completed' || item.status === 'error' ||
              item.status === 'stalled' || item.status === 'skipped') {
            state.activeDownloads.delete(item.jobId);
          } else {
            state.activeDownloads.set(item.jobId, {
              id: item.jobId,
              appName: item.name,
              bundleId: '',
              appId: item.appId,
              version: '',
              progress: item.progress || 0,
              bytesRead: item.bytesRead || 0,
              totalBytes: item.totalBytes || 0,
              speed: '',
              status: item.status,
              error: item.error || '',
              outputPath: item.outputPath || '',
              createdAt: Date.now() / 1000
            });
          }
          updateDownloadsBadge();
          renderActiveDownloadsTab();
        }
      });

      if (job.status === 'completed') {
        clearInterval(batchState.downloadPoll);
        showToast(batchText('batch_download_done_title'), 'success');

        // Queue everything that did not make it through so the user can retry
        // just those apps without touching the successful ones.
        batchState.retryable = (job.items || []).filter(isBatchRetryable)
          .map(i => ({ appId: i.appId, name: i.name, versionId: i.versionId || '' }));
        showBatchRetry(batchState.retryable.length);
      }
    } catch (err) {
      console.error('Batch download poll error:', err);
    }
  }, 500);
}

// ==========================================
// Install .IPA to a connected iOS device
// ==========================================

const installState = {
  devices: [],
  toolsAvailable: false,
  tools: [],
  currentJobId: null
};

let installPollTimer = null;

async function refreshInstallDevices() {
  const loading = document.getElementById('install-devices-loading');
  const empty = document.getElementById('install-devices-empty');
  const list = document.getElementById('install-devices-list');
  const select = document.getElementById('install-device-select');
  const warning = document.getElementById('install-tools-warning');
  const warningText = document.getElementById('install-tools-warning-text');

  if (loading) loading.style.display = 'block';
  if (empty) empty.style.display = 'none';
  if (list) list.innerHTML = '';

  try {
    const res = await fetch('/api/install/devices');
    const data = await res.json();
    if (loading) loading.style.display = 'none';

    if (!data.success) {
      showToast(data.message || batchText('install_devices_refresh_error'), 'error');
      if (empty) empty.style.display = 'block';
      return;
    }

    const devices = data.devices || [];
    installState.devices = devices;
    installState.toolsAvailable = !!data.toolsAvailable;
    installState.tools = data.tools || [];
    renderInstallDriverStatus(data.driver);

    // Refresh the device selector while preserving the prior selection.
    const previous = select ? select.getAttribute('data-udid') || select.value : '';
    if (select) {
      select.innerHTML = '';
      const placeholder = document.createElement('option');
      placeholder.value = '';
      placeholder.textContent = batchText('install_device_select_placeholder');
      select.appendChild(placeholder);
      devices.forEach(dev => {
        const opt = document.createElement('option');
        opt.value = dev.udid;
        const label = dev.modelName || dev.name || dev.udid;
        const desc = `${label}${dev.productVersion ? ` (iOS ${dev.productVersion})` : ''}${dev.name && dev.name !== dev.modelName ? ` — ${dev.name}` : ''}`;
        opt.textContent = desc;
        select.appendChild(opt);
      });
      if (devices.some(d => d.udid === previous)) {
        select.value = previous;
        select.setAttribute('data-udid', previous);
      }
    }

    if (list) {
      devices.forEach(dev => {
        const card = document.createElement('div');
        card.className = 'install-device-card' + (select && select.value === dev.udid ? ' selected' : '');
        const label = dev.modelName || dev.name || batchText('install_device_select_placeholder');
        const nameSuffix = dev.name && dev.name !== dev.modelName ? dev.name : '';
        const version = dev.productVersion ? (dev.productVersion || '') : '';
        card.innerHTML = `
          <div class="install-device-icon">📱</div>
          <div class="install-device-meta">
            <div class="install-device-name">${batchEscapeHtml(label)}${nameSuffix ? ` <span class="install-device-model">${batchEscapeHtml(nameSuffix)}</span>` : ''}</div>
            <div class="install-device-udid">${batchEscapeHtml(dev.udid)}</div>
            <div class="install-device-detail">
              ${version ? `iOS ${batchEscapeHtml(version)}` : ''}
              ${dev.modelName && dev.modelName !== dev.name ? ` · ${batchEscapeHtml(dev.modelName)}` : ''}
            </div>
          </div>
        `;
        card.addEventListener('click', () => {
          if (select) {
            select.value = dev.udid;
            select.setAttribute('data-udid', dev.udid);
          }
          document.querySelectorAll('.install-device-card').forEach(c => c.classList.remove('selected'));
          card.classList.add('selected');
        });
        list.appendChild(card);
      });
    }

    if (devices.length === 0) {
      if (empty) empty.style.display = 'block';
    }

    if (warning && warningText) {
      if (installState.toolsAvailable) {
        warning.style.display = 'none';
      } else {
        warning.style.display = 'flex';
        warningText.textContent = batchText('install_tools_missing_desc');
      }
    }
  } catch (err) {
    if (loading) loading.style.display = 'none';
    if (empty) empty.style.display = 'block';
    showToast(batchText('install_devices_refresh_error'), 'error');
  }
}

function updateInstallFileChosen() {
  const input = document.getElementById('install-file-input');
  const text = document.getElementById('install-drop-text');
  const icon = document.getElementById('install-drop-icon');
  if (!input || !text) return;

  const file = input.files && input.files[0];
  if (file) {
    text.textContent = `${batchText('install_file_selected')} ${file.name}`;
    if (icon) icon.textContent = '✅';
  } else {
    text.textContent = batchText('install_dropzone_text');
    if (icon) icon.textContent = '📦';
  }
}

function renderInstallDriverStatus(driver) {
  const card = document.getElementById('install-driver-card');
  const iconEl = document.getElementById('install-driver-icon');
  const titleEl = document.getElementById('install-driver-title');
  const textEl = document.getElementById('install-driver-text');
  const linksEl = document.getElementById('install-driver-links');
  if (!card) return;

  const dict = i18n[state.lang] || i18n.ru;
  const links = [];

  if (!driver || driver.required === false) {
    // macOS/Linux or unknown state: Apple Mobile Device Support is not needed.
    card.style.display = 'none';
    return;
  }

  card.style.display = 'flex';
  if (iconEl) iconEl.textContent = driver.installed ? '✅' : '⚠️';
  if (titleEl) titleEl.textContent = dict.install_driver_check_title;
  if (textEl) textEl.textContent = driver.installed ? dict.install_driver_ok : dict.install_driver_missing;

  if (linksEl) {
    linksEl.innerHTML = '';
    if (!driver.installed && driver.downloadUrl) {
      const a1 = document.createElement('a');
      a1.href = driver.downloadUrl;
      a1.target = '_blank';
      a1.rel = 'noopener';
      a1.className = 'btn btn-outline btn-sm';
      a1.textContent = '⬇️ ' + dict.install_driver_download;
      linksEl.appendChild(a1);
    }
    if (!driver.installed && driver.itunesUrl) {
      const a2 = document.createElement('a');
      a2.href = driver.itunesUrl;
      a2.target = '_blank';
      a2.rel = 'noopener';
      a2.className = 'btn btn-outline btn-sm';
      a2.textContent = '🍏 ' + dict.install_driver_itunes;
      linksEl.appendChild(a2);
    }
  }
}

async function handleInstallSubmit(e) {
  e.preventDefault();

  const select = document.getElementById('install-device-select');
  const fileInput = document.getElementById('install-file-input');
  const udid = select ? select.value : '';
  const deviceName = select && select.selectedIndex >= 0 ? select.options[select.selectedIndex].textContent : '';
  const file = fileInput && fileInput.files && fileInput.files[0];

  if (!udid) {
    showToast(batchText('install_needs_device'), 'error');
    return;
  }
  if (!file) {
    showToast(batchText('install_needs_file'), 'error');
    return;
  }

  const btn = document.getElementById('install-submit-btn');
  if (btn) {
    btn.disabled = true;
    btn.textContent = '⏳ ...';
  }

  const formData = new FormData();
  formData.append('udid', udid);
  formData.append('deviceName', deviceName);
  formData.append('file', file, file.name);

  try {
    const res = await fetch('/api/install/upload', {
      method: 'POST',
      body: formData
    });
    const data = await res.json();

    if (!data.success) {
      showToast(data.message || batchText('install_error'), 'error');
      return;
    }

    installState.currentJobId = data.jobId;
    showInstallProgressCard(deviceName, file.name);
    pollInstallJob(data.jobId);
    showToast(batchText('install_started_toast'), 'success');
  } catch (err) {
    showToast(batchText('install_error'), 'error');
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerHTML = `📲 <span>${batchText('install_button')}</span>`;
    }
  }
}

function showInstallProgressCard(deviceName, fileName) {
  const card = document.getElementById('install-progress-card');
  const subtitle = document.getElementById('install-progress-subtitle');
  const fill = document.getElementById('install-progress-fill');
  const percent = document.getElementById('install-progress-percent');
  const message = document.getElementById('install-progress-message');
  const log = document.getElementById('install-progress-log');

  if (card) card.style.display = 'block';
  if (subtitle) subtitle.textContent = `${deviceName || ''} · ${fileName || ''}`.trim();
  if (fill) {
    fill.classList.remove('installing');
    fill.style.width = '0%';
  }
  if (percent) percent.textContent = '0%';
  if (message) message.textContent = batchText('install_status_queued');
  if (log) log.textContent = '';

  if (card) card.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function pollInstallJob(jobId) {
  if (installPollTimer) clearInterval(installPollTimer);

  const fill = document.getElementById('install-progress-fill');
  const percent = document.getElementById('install-progress-percent');
  const message = document.getElementById('install-progress-message');
  const log = document.getElementById('install-progress-log');
  const subtitle = document.getElementById('install-progress-subtitle');

  installPollTimer = setInterval(async () => {
    try {
      const res = await fetch(`/api/install/status?jobId=${jobId}`);
      const job = await res.json();
      if (!job.id) {
        clearInterval(installPollTimer);
        installPollTimer = null;
        return;
      }

      if (log && job.log) log.textContent = job.log;

      if (job.status === 'completed') {
        clearInterval(installPollTimer);
        installPollTimer = null;
        if (fill) {
          fill.classList.remove('installing');
          fill.style.width = '100%';
        }
        if (percent) percent.textContent = '100%';
        if (message) message.textContent = batchText('install_completed');
        if (subtitle) subtitle.textContent = `${job.deviceName || job.udid || ''} · ${job.fileName || ''}`.trim();
        showToast(batchText('install_completed'), 'success');
      } else if (job.status === 'error') {
        clearInterval(installPollTimer);
        installPollTimer = null;
        if (fill) {
          fill.classList.remove('installing');
          fill.style.width = '100%';
        }
        if (percent) percent.textContent = '—';
        if (message) message.textContent = job.error || batchText('install_error');
        showToast(job.error || batchText('install_error'), 'error');
      } else {
        if (fill) {
          fill.classList.add('installing');
          fill.style.width = '100%';
        }
        if (percent) percent.textContent = '…';
        const label = batchText(`install_status_${job.status}`) || job.status;
        if (message) message.textContent = job.message || label;
      }
    } catch (err) {
      console.error('Install poll error:', err);
    }
  }, 700);
}

// ==========================================
// Initialization
// ==========================================

document.addEventListener('DOMContentLoaded', () => {
  applyTheme();
  applyLanguage();
  fetchStatus();
  checkICloudStatus();
  renderDownloadsTab();
  refreshActiveDownloads();

  // Search input typing listeners
  const searchInput = document.getElementById('search-input');
  const clearBtn = document.getElementById('search-clear-btn');
  if (searchInput && clearBtn) {
    searchInput.addEventListener('input', () => {
      clearBtn.style.display = searchInput.value.length > 0 ? 'block' : 'none';
    });
  }

  // Batch mass download listeners
  const batchForm = document.getElementById('batch-form');
  if (batchForm) batchForm.addEventListener('submit', handleBatchCheck);
  const batchFileInput = document.getElementById('batch-file-input');
  if (batchFileInput) batchFileInput.addEventListener('change', handleBatchFileChange);
  const batchPasteArea = document.getElementById('batch-paste-area');
  if (batchPasteArea) batchPasteArea.addEventListener('input', updateBatchParsedSummary);
});
