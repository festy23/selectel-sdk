# Discovery оставшихся API

Срез #73 закрепляет источники и неизвестные границы, не реализует remote
operations. Поставляемый [отчёт](../../../catalog/contracts/service-discovery.json)
содержит URI, content revisions, locators, факты и ограничения поиска на
2026-10-16. Source snapshots закреплены hashes; upstream reference не является
доказательством Selectel deployment или прав пользователя.

| Семейство | Подтверждено | Остаётся / владелец |
|---|---|---|
| Heat | Selectel matrix: Zed, API 1.0; upstream Heat 19.0.0 reference | user endpoint/auth/policy, полный inventory: [#124](https://github.com/festy23/selectel-sdk/issues/124) |
| Placement | Selectel matrix: Zed, API 1.0, max microversion 1.39; upstream 8.0.0 reference | user policy, endpoint, per-operation microversions/inventory: [#125](https://github.com/festy23/selectel-sdk/issues/125) |
| Gnocchi | Selectel matrix: release 4.5, API 1.0; upstream 4.5.0 reference | telemetry access, endpoint/auth, inventory: [#126](https://github.com/festy23/selectel-sdk/issues/126) |
| KMS | Публичный контракт не установлен | первичный публичный contract либо явно unresolved boundary: [#127](https://github.com/festy23/selectel-sdk/issues/127) |
| Hive Metastore | release 4.2.0; private-subnet Thrift 9083 / Iceberg REST HTTP 8080 | protocol/version/auth, inventory и безопасный transport: [#128](https://github.com/festy23/selectel-sdk/issues/128) |

Все пять записей остаются `discovery_pending`, inventory неизвестен. Новые
задачи являются native subissues #73 и blockers финальной приёмки #76. Наличие
компонента не подтверждает доступность каждой upstream операции. Отказ доступа,
отсутствие URI в API index или результата поиска не доказывает withdrawn API.
KMS не приравнивается к Secrets Manager/Barbican; Placement не дублирует Nova
server groups #31; Gnocchi не дублирует Prometheus #60. Для Hive не разрешаются
неявное отключение TLS, придуманный HTTPS gateway или Thrift поверх HTTP seam.

API index, URL map, OpenStack matrix и bounded sitemap/product-document search
не дают полного множества API. VDS, ClearML ML platform, Trino CLI и Redash GUI
сохранены как candidates в отчёте; product/CLI presence не создаёт Service ID
или вымышленные control-plane operations. Их публичные API границы требуют
дальнейшего исследования. Общий `coverage_ratio` остаётся `null`.

File Storage остаётся у #70/#74/#76, AI у #72/#74/#76; VMware у #68 и #107–#109,
legacy у #71 и #116–#118. Отчёт содержит ссылки на этих владельцев, не копирует
их независимые unmerged bindings и не увеличивает coverage aliases.

Проверки этого среза локальные: public Client catalog metadata, immutable copies,
content-bound artifact и отсутствие credential/HTTP I/O. Они не являются wire
или live проверками provider operations.
