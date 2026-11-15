# Notes

**Notes** est une application de prise de notes en markdown, un peu à la manière d'Obsidian : des notes rangées dans des dossiers, reliées par leurs `[[liens]]` et leurs `#tags`.

Un serveur Go sur PostgreSQL sert l'API et l'app web. Une CLI et un serveur MCP permettent aux scripts et aux agents d'utiliser cette API. L'app web est faite pour être ouverte depuis un téléphone ou une tablette, par exemple via Tailscale.

- **Dépôt :** https://github.com/cyril-fl/notes

## Documentation

Le [wiki](https://github.com/cyril-fl/notes/wiki) détaille l'app web écran par écran, les titres et les tags, les thèmes, les données et l'historique des versions.

Chaque partie a aussi son README :

- [`notes-go/`](./notes-go/README.md) : le serveur `notes-server`, la CLI `notes`, le MCP, l'API, la configuration et la sauvegarde.
- [`notes-app/`](./notes-app/README.md) : l'app web, ses raccourcis et ses réglages.

## Fonctionnalités

- **Notes et dossiers** : un éditeur markdown riche (avec un mode source), des dossiers en arbre, l'épinglage, les favoris et une corbeille.
- **Liens** : des `[[liens]]`, des rétroliens, l'inclusion d'une note dans une autre (`![[Note]]`), un graphe des notes et la liste des liens cassés.
- **Tags** : des `#tags` écrits dans le texte, avec une page par tag.
- **Recherche** : une palette ⌘K qui cherche dans les titres et le texte, des dossiers intelligents, et le remplacement d'un texte dans une note ou dans toutes.
- **Historique** : les versions de chaque note, qu'on peut comparer, restaurer ou copier en une nouvelle note, puis fusionner dans l'original.
- **Images et fichiers** : stockés dans Silo (compatible S3).
- **Comptes** : plusieurs comptes par serveur, sur invitation, des jetons d'API, et des notes chiffrées en base.
- **Personnalisation** : des thèmes, le mode clair ou sombre, des snippets, une barre latérale réglable et trois formules (Standard, Avancé, Ultra). Les réglages suivent le compte d'un appareil à l'autre.
- **Import et export** : un zip de fichiers markdown.

## Installation et utilisation

Notes est développé avec la dernière version de Go. En local, la stack tourne dans Docker grâce à [`notes-go/compose.yaml`](./notes-go/compose.yaml).

```bash
git clone https://github.com/cyril-fl/notes.git
cd notes
cp notes-go/.env.example notes-go/.env   # puis renseigner les mots de passe
make -C notes-go setup                   # installe les hooks git (lefthook)
make up                                  # Postgres, Silo et l'API dans Docker, l'app web sur :3000
echo '<mot de passe>' | docker exec -i note-api /notes-server user add <nom> --password-stdin
```

L'app web répond sur http://localhost:3000 (ouvrir `localhost`, pas `127.0.0.1`), et ses logs vont dans `.build/notes-app.log`. Pour l'arrêter, `make down` (Colima reste démarré).

Pour la CLI, sur le Mac :

```bash
go install github.com/cyril-fl/notes/notes-go/cmd/notes@latest
notes login https://notes.example.ts.net
```

### Prérequis

- [Go](https://go.dev/) (version indiquée dans `notes-go/go.mod`)
- [Bun](https://bun.sh/), pour l'app web
- [Docker](https://www.docker.com/) et [Docker Compose](https://docs.docker.com/compose/)
- [Colima](https://github.com/abiosoft/colima), utilisé par `make up` sur macOS
- [Plum](https://github.com/cyril-fl/plum), une bibliothèque utilitaire (CLI, configuration, process)

### Environnement Docker

[`notes-go/compose.yaml`](./notes-go/compose.yaml) définit les services du développement :

- `postgres` (`note-postgres`) : PostgreSQL 18, sur `127.0.0.1:5432`.
- `silo` (`note-silo`) : le stockage des images, sur `127.0.0.1:9000` (console sur `9001`).
- `api` (`note-api`) : `notes-server serve`, sur http://localhost:8080 (profil `api`, lancé par `make up`).
- `libredb` (`note-libredb`) : LibreDB Studio, une interface web pour explorer la base, sur http://localhost:3001.
- `app` (`note-app`) : un conteneur Go, pour lancer du Go contre la stack.

Les variables d'environnement sont décrites dans la section *Configuration* du [README de `notes-go`](./notes-go/README.md#configuration).

## Serveur

Chaque tag `v*` publie l'image `ghcr.io/cyril-fl/notes` (amd64 et arm64), qui sert l'API et l'app web sur la même origine. Sur le serveur, il faut [`compose.prod.yaml`](./compose.prod.yaml) et, à côté, un `.env` copié de [`.env.prod.example`](./.env.prod.example) :

```sh
docker compose -f compose.prod.yaml up -d                                    # Postgres, Silo, Notes et LibreDB Studio
echo '<mot de passe>' | docker exec -i note-api /notes-server user add <nom> --password-stdin
docker compose -f compose.prod.yaml pull && docker compose -f compose.prod.yaml up -d   # mise à jour
```

L'image ne contient que `notes-server`, le seul binaire qui parle à Postgres et à Silo. Il fait tourner l'API et l'app web, et porte les commandes de l'admin (`user add`, `user reset-password`, `rotate-secret`), qu'on lance dans le conteneur avec `docker exec`. La CLI `notes` est un client de l'API.

Notes écoute en HTTP sur `NOTES_LISTEN` (`127.0.0.1:8080` par défaut). Le cookie de session exige HTTPS : c'est le proxy placé devant qui le fournit. `WEB_ORIGIN` est l'adresse exacte que tape le navigateur. LibreDB Studio écoute sur `LIBREDB_LISTEN` (`127.0.0.1:3001`) : on s'y connecte à `note-postgres:5432` avec l'utilisateur `POSTGRES_USER`.

Au démarrage, Notes ajoute à la base les tables et les colonnes des nouvelles versions. Un renommage ou un changement de type reste à faire à la main ; le message du commit l'indique.

> ⚠️ `NOTES_SECRET` chiffre les notes. Gardez-en une copie dans un gestionnaire de mots de passe, jamais dans une sauvegarde : sans cette clé, les notes sont illisibles.

### Comptes

Le premier compte de la base est admin. Lui seul invite, liste et supprime les autres, depuis les réglages de l'app (section *Utilisateurs*). Ce premier compte se crée de trois façons :

- sur une base vide, l'app mène droit à la page de création de compte ;
- si `NOTES_HOST_NAME` et `NOTES_HOST_PASSWORD` sont renseignés dans le `.env`, Notes le crée au démarrage ; ces deux lignes sont ensuite à vider ;
- la commande `notes-server user add` ci-dessus marche dans tous les cas.

Ensuite, `NOTES_ADD_USER_MODE` décide qui peut créer un compte :

- `invitation` (par défaut) : l'admin crée un lien d'invitation dans les réglages et l'envoie. Le lien sert une seule fois et expire au bout de 7 jours. C'est l'invité qui choisit son nom et son mot de passe. Hors invitation, la page de création de compte est fermée.
- `public` : n'importe qui atteignant l'app peut créer un compte. Ce mode est à réserver à un réseau de confiance (Tailscale), car rien ne limite le nombre de créations.

Supprimer un compte efface pour de bon ses notes, dossiers, tags et images. Un admin ne peut ni se supprimer lui-même, ni supprimer le dernier admin.

## Makefile

À la racine :

| Cible       | Description                                                                   |
|-------------|-------------------------------------------------------------------------------|
| `make up`   | Démarre Colima, Postgres, Silo et l'API dans Docker, puis l'app web sur :3000 |
| `make down` | Arrête l'app web et les conteneurs (Colima reste démarré)                     |
| `make e2e`  | Lance les tests navigateur (Playwright) contre la stack                       |
| `make perf` | Mesure l'API et les pages sur un compte de 50 000 notes générées              |

Dans `notes-go/` :

| Cible          | Description                                                                                    |
|----------------|------------------------------------------------------------------------------------------------|
| `make setup`   | Installe les hooks git (lefthook)                                                              |
| `make test`    | Lance les tests Go dans la base `<POSTGRES_DB>_test` (`ARGS=./app/...` pour cibler un dossier) |
| `make api`     | Recompile et redémarre le conteneur de l'API                                                   |
| `make openapi` | Régénère `docs/swagger.yaml` (puis `bun run types` dans `notes-app/`)                          |
| `make backup`  | Sauvegarde la base et les images dans `backups/`                                               |
| `make pgsql`   | Ouvre psql, ou lance une requête avec `QUERY="..."`                                            |
| `make connect` | Connecte la CLI à une API (`TO=local` ou `TO=fedora`)                                          |

## Qualité

- Les hooks lefthook (`make setup` dans `notes-go/`) lancent gofmt, Prettier et actionlint (pour les workflows) avant chaque commit. Ils vérifient aussi le préfixe du message (`feat:`, `fix:`…).
- La CI GitHub Actions ([`.github/workflows/ci.yml`](./.github/workflows/ci.yml)) tourne à chaque push et à chaque PR :
  - le format, vet et les tests Go ;
  - le lint, le format, les tests, le typecheck et le build de l'app ;
  - la vérification que `swagger.yaml` et `api.d.ts` sont à jour ;
  - les e2e sur la stack Docker ;
  - le build de l'image serveur, sans la publier.

## Remerciements

### Bibliothèques

- [Cobra](https://github.com/spf13/cobra) : la CLI
- [Bun](https://bun.uptrace.dev/) : l'ORM, sur PostgreSQL
- [MinIO Go Client](https://github.com/minio/minio-go) : le stockage S3
- [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) : le serveur MCP
- [Nuxt](https://nuxt.com/) et [Nuxt UI](https://ui.nuxt.com/) : l'app web
- [Tiptap](https://tiptap.dev/) : l'éditeur
- [KaTeX](https://katex.org/) et [Mermaid](https://mermaid.js.org/) : les formules et les diagrammes

### Services

- Silo : le stockage des images, compatible S3
- LibreDB Studio : une interface web pour la base

## Licence

[MIT](./notes-go/LICENSE)
