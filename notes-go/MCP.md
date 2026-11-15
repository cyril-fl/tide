# MCP : donner les notes à un agent

`notes mcp` est la troisième façade sur les notes, à côté de la CLI et de l'API : un serveur MCP qui
donne à un agent (Claude Code, Claude Desktop, tout client MCP) des outils pour chercher, lire,
écrire, ranger et supprimer les notes d'un compte, avec leurs fichiers, leur historique et leurs liens.

Ce n'est pas une API HTTP : il n'écoute sur aucun port et n'a pas de routes. C'est un client de l'API,
comme le reste de la CLI : chaque outil devient une requête au serveur, avec le jeton de la session. Les trois points à comprendre sont donc ailleurs : **comment l'agent le lance**,
**pour quel compte il agit**, et **ce qu'il peut faire**.

## 1. Comment l'agent le lance

Le transport est **stdio** : le client MCP démarre `notes mcp` comme sous-processus et échange du
JSON-RPC sur son entrée et sa sortie standard. Rien d'autre ne doit être écrit sur stdout, ce qui
casserait le dialogue.

Conséquence : **on ne lance pas `notes mcp` à la main** pour s'en servir. C'est le client qui le
démarre, l'arrête et parle avec lui. Il faut donc seulement le déclarer chez le client, et le faire
tourner là où le serveur est joignable (voir « Vérifier que ça marche » pour le tester sans client).

### Le binaire `notes`

`go install github.com/cyril-fl/notes/notes-go/cmd/notes@latest` l'installe dans `$(go env GOPATH)/bin`.
Chez un client MCP, donne-le en chemin absolu : un client lancé depuis le Finder n'a pas le `PATH` du
shell.

```sh
echo "$(go env GOPATH)/bin/notes"   # le chemin à donner au client
```

### L'environnement : en général, n'en donner aucun

`notes mcp` n'a besoin que de la session de `notes login` (section 2) : ni base, ni `NOTES_SECRET`, ni
Silo de son côté, tout passe par le serveur. Si `notes whoami` répond dans un terminal, le MCP
marchera **sans aucune variable** chez le client :

```sh
claude mcp add notes -- "$(go env GOPATH)/bin/notes" mcp
```

Codex (`~/.codex/config.toml`) :

```toml
[mcp_servers.notes]
command = "/Users/<moi>/go/bin/notes"   # le chemin donné par la commande plus haut
args = ["mcp"]
```

`.mcp.json` ou Claude Desktop :

```json
{
  "mcpServers": {
    "notes": { "command": "/Users/<moi>/go/bin/notes", "args": ["mcp"] }
  }
}
```

**Le piège : une variable donnée par le client gagne toujours sur `~/.config/notes/session.json`.**
Un bloc `env` recopié d'une autre installation, avec un `NOTES_URL` ou un `NOTES_SESSION`, envoie
donc le MCP vers un autre serveur ou un autre compte, même si la session de la machine est juste.

Les variables de la base (`POSTGRES_*`, `PORT`, `NOTES_SECRET`) et de Silo (`S3_*`) ne servent plus à
`notes mcp` : elles ne regardent que le serveur et les commandes d'admin.

## 2. Pour quel compte il agit

Il n'y a **ni port, ni jeton, ni mot de passe à passer** dans sa configuration. `notes mcp` parle à
l'API comme le reste de la CLI, au serveur et pour le compte de la **session CLI de la machine** —
l'URL et le jeton écrits par `notes login <url>` dans `~/.config/notes/session.json`, en `0600`.

L'agent n'est donc jamais « connecté » à quoi que ce soit, et ne voit ni mot de passe ni jeton : il
appelle `create_note`, et le serveur agit déjà pour le compte. La seule chose à faire est un
`notes login`, **une fois**, sur la machine et sous l'utilisateur Unix qui lancera l'agent :

```sh
notes login https://notes.example.ts.net
```

- La session est **par utilisateur Unix**, pas par machine : si l'agent tourne sous un utilisateur de
  service ou sous `root`, c'est sous celui-là qu'il faut se connecter.
- Elle dure **30 jours glissants**, chaque usage la relance : un agent qui s'en sert régulièrement ne
  la perd pas. Expirée, les outils répondent `not logged in, or the session expired`, et il suffit de
  refaire `notes login <url>`. Sans session du tout, `notes mcp` ne démarre pas
  (`not logged in: run notes login <url>`) et le client l'annonce en échec.
- `notes logout` la supprime : à ne pas lancer sur une machine dont un agent dépend.

### Quand on ne peut pas se connecter là où l'agent tourne

Un agent dans un conteneur ou hébergé ailleurs n'a pas de shell pour `notes login`, ni de home
persistant pour le fichier. Il prend alors le jeton dans **`NOTES_SESSION`** et le serveur dans
**`NOTES_URL`**, lues avant le fichier (le fichier reste le cas normal). Le jeton se demande là où l'on peut se connecter :

```sh
notes login https://notes.example.ts.net --print-token
```

Il crée une session **à part**, l'affiche et n'écrit rien : celle de la machine n'est pas touchée.
Le formulaire s'affiche sur stderr, stdout ne porte que le jeton : `NOTES_SESSION=$(notes login <url>
--print-token)` le range directement dans une variable, sans qu'il passe par l'écran. Le
jeton n'est lié à aucune machine (la base n'en garde que le hash), il se donne donc à l'agent par son
environnement :

```sh
NOTES_URL=https://notes.example.ts.net NOTES_SESSION='<le-jeton>' notes mcp
```

- Elle vit comme les autres : **30 jours glissants**, relancée à chaque usage.
- Pour la couper : `NOTES_SESSION='<le-jeton>' notes logout`. Seule cette session part, le fichier
  de la machine reste.

**Ce jeton ouvre le compte entier.** Il se traite comme un mot de passe : il ne va **pas** dans un
`.mcp.json`, un `config.toml` ou tout fichier versionné, ni dans le bloc `env` d'un client dont la
configuration se partage. Il se donne par le gestionnaire de secrets de la plateforme qui héberge
l'agent (secret de conteneur, variable chiffrée du CI), ou par l'environnement du shell qui lance le
client.

### Ce que ça implique

Tout agent capable de lancer `notes mcp` sur cette machine a **tous les droits du compte connecté**,
écriture et suppression comprises. La garantie est celle de l'utilisateur Unix, exactement comme pour
`notes show note`. Il n'y a ni compte réservé aux agents, ni droits en lecture seule, ni partage
entre comptes : c'est le compte, en entier.

## 3. Ce qu'il peut faire

À la connexion, l'agent reçoit les vingt-trois outils avec leur description et leur schéma, puis travaille
en langage naturel : « range mes notes de réunion de novembre dans /Travail/Réunions » devient un
`search_notes`, un `create_folder`, puis une suite de `move_item`.

| Outil | Ce qu'il fait |
|---|---|
| `search_notes` | Les notes dont le titre ou le contenu contient un texte, avec la ligne trouvée : les 50 premières, ou `limit` (0 pour toutes) |
| `recent_notes` | Les notes modifiées le plus récemment, la plus récente d'abord |
| `read_note` | Le contenu d'une note, son chemin et ses tags |
| `list_folder` | Les notes et dossiers directement dans un dossier |
| `tags` | Les notes portant un tag, avec leur chemin ; sans tag, les tags du compte, les plus utilisés d'abord |
| `create_note` | Une note à partir de markdown : le premier titre devient le titre, les `#hashtags` des tags |
| `edit_note` | Remplace **tout** le contenu d'une note ; refuse une note verrouillée |
| `create_folder` | Un dossier, à la racine ou dans un autre |
| `rename_folder` | Le titre d'un dossier |
| `move_item` | Déplace une note ou un dossier ; un dossier emmène son contenu |
| `delete_note` | Met une note à la corbeille |
| `delete_folder` | Met un dossier **et tout son contenu** à la corbeille |
| `list_trash` | Ce qui est à la corbeille, avec l'uuid que `restore_item` attend |
| `restore_item` | Ressort de la corbeille, avec ce qui y est parti en même temps |
| `rename_tag` | Renomme un tag sur toutes ses notes, dans leur texte aussi |
| `replace_notes` | Remplace un texte brut dans plusieurs notes d'un coup (`folder` pour limiter, `dry_run` pour compter avant) ; les notes verrouillées sont laissées, chaque note changée garde son texte dans l'historique |
| `delete_tag` | Retire un tag de toutes ses notes ; une note qui ne contenait que lui part à la corbeille |
| `note_history` | Les versions antérieures d'une note (sans `version`), ou le texte de l'une d'elles |
| `note_links` | Les liens d'une note, ses rétroliens et les notes qui partagent ses tags ; sans note, les liens cassés de tout le compte |
| `read_file` | Une image (que l'agent voit) ou une pièce jointe citée par une note ; sans fichier, la liste des fichiers |
| `attach_file` | Stocke un fichier envoyé en base64 et rend le markdown qui le cite, à poser avec `create_note` ou `edit_note` |
| `export_notes` | Un dossier, des éléments ou tout le compte en fichiers markdown, `[[liens]]` réécrits |
| `import_notes` | Des fichiers markdown en notes, leurs répertoires en dossiers, tout ou rien |

**Un outil par domaine, pas un par processus.** Chaque outil annoncé coûte du contexte à l'agent à
chaque connexion (environ 300 à 750 octets de nom, description et schéma d'entrée ; le schéma de
sortie, que les clients ne donnent en général pas au modèle, double à peu près ce chiffre). Lister et
lire passent donc par le même outil, un argument vide donnant la liste (`tags`, `note_history`,
`note_links`, `read_file`) : `list_tags` et `read_tag` sont devenus `tags`, et les treize processus
ajoutés ensuite tiennent en huit outils. La création d'un tag seul n'est pas exposée,
un tag sans note ne sert à rien à un agent.
`delete_note` et `delete_folder` restent séparés exprès : un chemin de dossier donné à `delete_note`
est refusé, au lieu d'envoyer tout un sous-arbre à la corbeille.

Ils passent par l'API, comme la CLI et l'app : une règle (titre, tags, liens, corbeille, historique,
chiffrement) s'applique pareil des trois côtés.

### Ce qu'il faut savoir avant de le lâcher dessus

- **Une note ou un dossier se désigne par son uuid ou par son chemin** depuis la racine
  (`/Dossier/Note`). Seul `restore_item` exige l'uuid : un élément à la corbeille n'a plus de chemin à
  résoudre. Un chemin qui désigne deux éléments est refusé (`several items at this path, use a uuid`).
- **`edit_note` écrase.** Rien n'est conservé de l'ancien contenu, l'agent doit lire la note avant. Le
  texte remplacé va dans l'historique de la note comme pour une modification dans l'app (à la première
  modification d'une session d'écriture, puis au moins une fois par heure), d'où `note_history` et
  l'app le ressortent. Les `#tags` du contenu restent, ceux de `tags` s'y ajoutent.
- **Une suppression passe par la corbeille**, d'où `restore_item` la ressort. Mais la corbeille se
  vide d'elle-même passé le délai de rétention du compte (`purgeitems`, lancé par le serveur), sauf si ce compte garde sa corbeille indéfiniment : ce n'est pas un
  filet garanti. Le vidage définitif, lui, n'est pas donné à
  l'agent : il reste à la CLI et à l'app.
- **Un agent réorganise vite.** `delete_folder` envoie tout un sous-arbre à la corbeille en un appel,
  et `move_item` ne demande aucune confirmation. Un client MCP qui demande validation avant chaque
  appel d'outil est la bonne façon de travailler au début. `make backup` avant une grosse
  réorganisation reste la règle.
- **Les fichiers passent par le serveur**, qui les range dans Silo. `attach_file` accepte jusqu'à
  20 Mo, `import_notes` un zip de 200 Mo. `read_file` rend le fichier entier en un bloc, une vidéo
  comprise. `export_notes` télécharge aussi les octets des images citées, pour n'en garder que l'id.
- **`delete_tag` et `rename_tag` réécrivent le texte des notes**, corbeille comprise, sans passer par
  l'historique : c'est le comportement de la CLI et de l'app.
- **Ce qui n'est pas exposé** : le graphe complet (`note_links` en donne le voisinage note par note),
  les plans, la gestion des comptes, la création d'un tag seul, le vidage de la corbeille.

## Vérifier que ça marche

Le test `app/cli/cmd-mcp/cmd_test.go` branche un client MCP en mémoire sur le serveur MCP, lui-même
client d'une vraie API sur la base de test. Il vérifie que les vingt-trois outils sont annoncés avec un
schéma et une description — ce que le SDK ne sait pas déduire seul d'une structure d'arguments mal
formée — puis les appelle un à un.

```sh
make test ARGS=./app/cli/cmd-mcp/
```

Côté client, `claude mcp list` dit si le serveur répond (Codex : `codex mcp list`).

Sans client, on peut lui parler à la main, dans l'environnement que le client lui donnera : la commande
ci-dessous lui envoie l'ouverture du dialogue puis un `recent_notes`, en lecture seule. Deux lignes de
JSON en retour : il marche. Rien, ou une erreur : c'est la session ou le serveur.

```sh
{ printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"recent_notes","arguments":{}}}'
  sleep 3; } | notes mcp
```

Le `sleep` garde l'entrée ouverte le temps des réponses : sans lui, le serveur s'arrête avant d'avoir
répondu.

Quand il échoue, dans l'ordre :

1. **La session** : `notes whoami` sous l'utilisateur Unix qui lance l'agent dit pour quel compte
   et quel serveur il agit. Rien : `notes login <url>`, ou `NOTES_URL` et `NOTES_SESSION`.
2. **Le client lui donne-t-il un bloc `env`** avec un `NOTES_URL` ou un `NOTES_SESSION` ? Ils gagnent
   sur le fichier : une valeur périmée masque une session valide (voir le piège, section 1).
3. **Le serveur est-il joignable** depuis la machine de l'agent (Tailscale allumé, URL juste) ?
