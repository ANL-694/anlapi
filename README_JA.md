# anlapi

![Go](https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3-42b883?logo=vuedotjs&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-4169E1?logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-7+-DC382D?logo=redis&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)
![License](https://img.shields.io/badge/License-LGPL--3.0-blue)

anlapi は Sub2API をベースに二次開発された、セルフホスト向けの AI API ゲートウェイ兼サブスクリプション管理プラットフォームです。アカウントプール、API Key 管理、複数プロバイダーへのリクエスト転送、利用量計測、サブスクリプション課金、モデレーション制御、管理運用機能を提供します。

[English](README.md) | [中文](README_CN.md) | 日本語

サイト：[https://api.anlmc.top](https://api.anlmc.top)

QQ グループ：`146499741`

このリポジトリは、プライベートデプロイ、カスタマイズ、二次開発を目的としています。本番環境のシークレット、プライベートサーバー設定、ホスティングサービスの認証情報、商用運用データは含まれていません。

## 重要な注意事項

デプロイまたは運用する前に、以下を必ず確認してください。

- 利用規約上のリスク：サブスクリプション型またはアカウント型の上流サービスを経由してリクエストを転送すると、一部の上流プロバイダーの利用規約に違反する可能性があります。使用前に関連する契約を確認してください。
- コンプライアンス：このプロジェクトは、利用する国または地域の法律・規制に従って使用してください。
- アカウントリスク：アカウント停止、クォータリセット、サービス中断、上流ポリシー変更、課金エラーは、運用者が対応すべき運用リスクです。
- 免責事項：このプロジェクトは技術学習、セルフホスティング、二次開発のために提供されています。デプロイ、データ、ユーザー、決済、上流アカウントについては利用者自身が責任を負います。

## 機能

- chat、responses、models、embeddings、image、ストリーミング用途に対応した OpenAI 互換ゲートウェイエンドポイント。
- Grok OAuth、Kiro OAuth、無料モデルプロバイダーの接続、設定可能なプライベートアカウント接続フローに対応。
- OpenAI 互換チャネルとアカウント型上流サービスに対応する複数プロバイダーのルーティング。
- 公開、プライベート、所有、相乗り型スケジューリング概念を含むアカウントプール管理。
- 複数グループルーティング、IP アクセス制御、クォータ制御、利用記録、課金メタデータを備えた API Key 管理。
- ユーザーサブスクリプション、チャージフロー、引換コード、招待報酬、ショップ/カードキー機能。
- ユーザー、アカウント、チャネル、決済、モデレーション、リスクイベント、データ管理、システム設定を管理する管理ダッシュボード。
- リクエスト/レスポンス監査のためのコンテンツモデレーションとリスク制御の統合ポイント。
- タグビルド、Docker イメージ、アーカイブ、GitHub Releases に対応した組み込みリリースワークフロー。
- Vue 3、TypeScript、Pinia、Vue Router、Tailwind CSS、Vite によるフロントエンドコンソール。
- Go、Gin、Ent、PostgreSQL、Redis とモジュール化されたサービス境界によるバックエンド。

## 1.0.8 更新内容

- Sub2API v0.1.164 の互換性修正として、Codex 一括インポート、GPT-5.6 の具体的なテストモデル、OAuth `input` 正規化、チャネルモデル名正規化を取り込みました。
- OpenAI ストリームが異常切断した場合、共有プロキシを後続スケジューリングから一時隔離します。正常終了やクライアント側キャンセルは隔離対象になりません。
- Grok の 402 応答時のクールダウンと、簡易モードで自動作成される Grok デフォルトグループの画像生成能力を追加しました。
- CC Switch の Grok Key を Grok Build に取り込み、モデル制限時刻の表示とブラウザーセッション値の監査マスキングを改善しました。
- ANL 独自の決済、残高、画像生成専用ルート、OAuth/API Key 分離、ユーザー単位の同時実行制御を維持します。複合グループ、Ollama Cloud 使用量、Alipay モバイルディープリンクはこの更新に含みません。

## 1.0.7 更新内容

- Sub2API v0.1.163 に追従し、グループ単位の OpenAI/Codex 推論強度上限と厳密マッピングを HTTP・WebSocket の両経路に追加しました。
- Grok `/responses/compact`、Codex クライアントツールの往復保持、モデル単位の 403 分離、キャッシュセッションを修正しました。
- 正常終了時のバッファ使用量、画像 Token 課金、フェイルオーバー課金、スケジューラキャッシュとクォータ情報を修正しました。
- Redis ACL ユーザー名設定と、モバイル表示、プラン有効期限、使用量フィルター、倍率表示の改善を取り込みました。
- ANL 独自の決済、残高表示、画像生成専用ルート、OAuth/API Key 分離、ユーザー単位の同時実行制御を維持しています。

## 1.0.6 更新内容

- Sub2API v0.1.162 の上流課金プローブ設定・一括プローブルートを補完し、管理画面のアカウントページで設定取得が 404 になる問題を修正しました。

## 1.0.5 更新内容

- 管理画面の同時実行カードを「ユーザー同時実行」に改め、現在の実使用数とユーザー上限のみを表示します。
- 狭い画面でヘッダーが横にはみ出す問題を修正し、右上の残高表示を維持します。極端に狭い画面では購読進捗のみを省略します。

## 1.0.4 更新内容

- Sub2API v0.1.162 の OpenAI/Codex、Responses、Anthropic、Grok メディア、サブスクリプション期限、非同期画像ストレージ関連の改善を取り込みました。
- API Key の IP 制限は、明示的に信頼済みプロキシと互換モードを設定した場合のみ転送ヘッダーを利用します。オリジンへの直接アクセスでは Cloudflare ヘッダーの偽装で制限を回避できません。
- リクエスト同時実行数はユーザー単位のみで制御します。アカウント、グループ、プラットフォーム、API Key を追加の同時実行数ゲートとして使用しません。
- 管理画面からアカウント容量、グループ容量、アカウント同時実行数の編集、アカウント待機キュー表示を削除しました。

## 1.0.3 更新内容

- バックエンドツールチェーンを Go 1.26.5 に更新し、ストレージ連携で利用する AWS SDK の脆弱性対応依存関係を更新。
- Grok OAuth、Kiro OAuth、K12 アカウントレベル対応を追加し、動画関連ゲートウェイエンドポイントを補強。
- 無料モデルプロバイダー接続、複数グループ API Key ルーティング、API Key IP アクセス制御を追加。
- 相乗りプール、プライベートアカウント、サブスクリプション、課金、推論 Token、利用統計の処理を改善。
- 現在の依存関係に合わせて CI、セキュリティスキャン、フロントエンド監査処理を更新。

## 技術スタック

- バックエンド：Go 1.26.5、Gin、Ent、PostgreSQL、Redis
- フロントエンド：Vue 3、TypeScript、Vite、Pinia、Tailwind CSS
- テスト：Go test、Vitest、vue-tsc、ESLint
- デプロイ：Docker またはソースビルド。外部 PostgreSQL と Redis の利用を推奨

## リポジトリ構成

```text
.
├── backend/              # Go バックエンド、マイグレーション、サービス、ハンドラー、リポジトリ
├── frontend/             # Vue 3 管理/ユーザーコンソール
├── deploy/               # デプロイ例と設定テンプレート
├── docs/                 # 追加の連携・運用ドキュメント
├── assets/               # 静的プロジェクトアセット
├── Makefile              # 共通のビルド・テスト入口
└── Dockerfile            # 本番イメージビルド
```

## 要件

- Go 1.26.5
- Node.js 20+
- pnpm 9+
- PostgreSQL
- Redis
- Docker（任意。ただしデプロイでは推奨）

## 設定

サンプル設定から開始します。

```bash
cp deploy/config.example.yaml deploy/config.yaml
```

生成された設定を環境に合わせて編集します。

- `server`：ホスト、ポート、フロントエンド URL、リクエストボディ制限、CORS、セキュリティヘッダー。
- `database`：PostgreSQL 接続設定。
- `redis`：キャッシュおよびキューバックエンド設定。
- `gateway`：上流タイムアウト、ボディサイズ制限、ルーティング、モデル挙動。
- `security`：URL allowlist、レスポンスヘッダーのフィルタリング、プロキシフォールバック、CSP。
- 必要に応じて payment、email、storage、moderation、OAuth セクションを設定します。

本番認証情報をコミットしないでください。ローカルおよびデプロイ固有の設定ファイルは git で無視されます。

## 開発

フロントエンド依存関係をインストールします。

```bash
pnpm --dir frontend install
```

フロントエンド開発サーバーを起動します。

```bash
pnpm --dir frontend run dev
```

ソースからバックエンドを実行します。

```bash
cd backend
go run ./cmd/server
```

初回起動時、有効な設定またはインストール状態が検出されない場合、バックエンドはセットアップフローを開始することがあります。

## ビルド

バックエンドとフロントエンドをビルドします。

```bash
make build
```

バックエンドのみをビルドします。

```bash
make build-backend
```

フロントエンドのみをビルドします。

```bash
make build-frontend
```

Docker イメージをビルドします。

```bash
docker build -t anlapi:local .
```

## テスト

設定済みのチェックをすべて実行します。

```bash
make test
```

バックエンドテストを実行します。

```bash
cd backend
go test -tags=unit ./...
go test -tags=integration ./...
```

フロントエンドチェックを実行します。

```bash
pnpm --dir frontend run lint:check
pnpm --dir frontend run typecheck
pnpm --dir frontend run i18n:audit:strict
pnpm --dir frontend exec vitest run
```

リポジトリ設定で golangci-lint を実行します。

```bash
cd backend
golangci-lint run ./... --timeout=30m
```

ローカルに `golangci-lint` がない場合は、CI と同じバージョンを使用できます。

```bash
cd backend
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0 run ./... --timeout=30m
```

## デプロイメモ

本番環境では、anlapi を Nginx、Caddy、マネージドロードバランサーなどのリバースプロキシの背後で実行することを推奨します。

### Nginx リバースプロキシに関する注意

Nginx を使用し、アカウントスケジューリング、sticky session、Codex CLI、またはアンダースコアを含むヘッダーを送信するクライアントを利用する場合は、Nginx の `http` ブロックで以下を有効化してください。

```nginx
underscores_in_headers on;
```

Nginx はデフォルトでアンダースコアを含むヘッダーを破棄します。これにより、セッションルーティングや一部のネイティブクライアント互換パスが壊れる可能性があります。

推奨される本番環境の基本事項：

- PostgreSQL と Redis はアプリケーションコンテナの外部で運用します。
- シークレットをイメージに焼き込まず、本番設定ファイルをマウントします。
- TLS はリバースプロキシまたはロードバランサーで終端します。
- `/api/*`、`/v1/*`、ストリーミング、ゲートウェイルートを CDN キャッシュ対象にしないでください。
- リバースプロキシとバックエンドでリクエストボディ制限を一致させます。
- マイグレーションまたはアプリケーションアップグレード前に PostgreSQL をバックアップしてください。

## セキュリティ

- API Key、OAuth secret、決済キー、データベースパスワード、サーバー認証情報をコミットしないでください。
- サービスを公開する前に `deploy/config.example.yaml` を確認してください。
- 強力なパスワード、利用可能であれば MFA、信頼できるリバースプロキシルールで管理画面へのアクセスを制限してください。
- 決済、ストレージ、モデレーション、メール認証情報には最小限の権限のみを付与してください。
- 変更を公開する前に `make secret-scan` を実行してください。

## ライセンス

このプロジェクトは [LICENSE](LICENSE) に含まれるライセンスに従います。

## 謝辞

anlapi は Sub2API をベースに構築され、セルフホスト AI ゲートウェイ、サブスクリプション、会計、運用ワークフロー向けに拡張されています。

- [PIXEL-API/PixelAPI](https://github.com/PIXEL-API/PixelAPI)
- [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api)
