# MCP : donner les notes à un agent

`tide mcp` est la troisième façade sur les notes, à côté de la CLI et de l'API : un serveur MCP qui
donne à un agent (Claude Code, Claude Desktop, tout client MCP) des outils pour chercher, lire,
écrire, ranger et supprimer les notes d'un compte, avec leurs fichiers, leur historique et leurs liens.

`tide mcp` n'écoute sur aucun port et n'a pas de routes. C'est un client de l'API,
comme le reste de la CLI : chaque outil devient une requête au serveur, avec le jeton de la session.
Pour un agent qui ne peut pas lancer de commande, l'app Claude sur téléphone ou ChatGPT par exemple, le serveur sert
les mêmes outils en HTTP, authentifiés par un jeton d'API ou une connexion OAuth : section 4. Les trois points à comprendre sont donc ailleurs : **comment l'agent le lance**,
**pour quel compte il agit**, et **ce qu'il peut faire**.

## 1. Comment l'agent le lance

Le transport est **stdio** : le client MCP démarre `tide mcp` comme sous-processus et échange du
JSON-RPC sur son entrée et sa sortie standard. Rien d'autre ne doit être écrit sur stdout, ce qui
casserait le dialogue.

Conséquence : **on ne lance pas `tide mcp` à la main** pour s'en servir. C'est le client qui le
démarre, l'arrête et parle avec lui. Il faut donc seulement le déclarer chez le client, et le faire
tourner là où le serveur est joignable (voir « Vérifier que ça marche » pour le tester sans client).

### Le binaire `tide`

`go install github.com/cyril-fl/tide/cmd/tide@latest` l'installe dans `$(go env GOPATH)/bin`.
Chez un client MCP, donne-le en chemin absolu : un client lancé depuis le Finder n'a pas le `PATH` du
shell.

```sh
echo "$(go env GOPATH)/bin/tide"   # le chemin à donner au client
```

### L'environnement : en général, n'en donner aucun

`tide mcp` n'a besoin que de la session de `tide login` (section 2) : ni base, ni `TIDE_SECRET`, ni
Silo de son côté, tout passe par le serveur. Si `tide whoami` répond dans un terminal, le MCP
marchera **sans aucune variable** chez le client :

```sh
claude mcp add tide -- "$(go env GOPATH)/bin/tide" mcp
```

Codex (`~/.codex/config.toml`) :

```toml
[mcp_servers.tide]
command = "/Users/<moi>/go/bin/tide"   # le chemin donné par la commande plus haut
args = ["mcp"]
```

`.mcp.json` ou Claude Desktop :

```json
{
  "mcpServers": {
    "tide": { "command": "/Users/<moi>/go/bin/tide", "args": ["mcp"] }
  }
}
```

**Le piège : une variable donnée par le client gagne toujours sur `~/.config/tide/session.json`.**
Un bloc `env` recopié d'une autre installation, avec un `TIDE_URL` ou un `TIDE_SESSION`, envoie
donc le MCP vers un autre serveur ou un autre compte, même si la session de la machine est juste.

Les variables de la base (`POSTGRES_*`, `PORT`, `TIDE_SECRET`) et de Silo (`S3_*`) ne servent plus à
`tide mcp` : elles ne regardent que le serveur et les commandes d'admin.

## 2. Pour quel compte il agit

Il n'y a **ni port, ni jeton, ni mot de passe à passer** dans sa configuration. `tide mcp` parle à
l'API comme le reste de la CLI, au serveur et pour le compte de la **session CLI de la machine** —
l'URL et le jeton écrits par `tide login <url>` dans `~/.config/tide/session.json`, en `0600`.

L'agent n'est donc jamais « connecté » à quoi que ce soit, et ne voit ni mot de passe ni jeton : il
appelle `create_note`, et le serveur agit déjà pour le compte. La seule chose à faire est un
`tide login`, **une fois**, sur la machine et sous l'utilisateur Unix qui lancera l'agent :

```sh
tide login https://notes.example.ts.net
```

- La session est **par utilisateur Unix**, pas par machine : si l'agent tourne sous un utilisateur de
  service ou sous `root`, c'est sous celui-là qu'il faut se connecter.
- Elle dure **30 jours glissants**, chaque usage la relance : un agent qui s'en sert régulièrement ne
  la perd pas. Expirée, les outils répondent `not logged in, or the session expired`, et il suffit de
  refaire `tide login <url>`. Sans session du tout, `tide mcp` ne démarre pas
  (`not logged in: run tide login <url>`) et le client l'annonce en échec.
- `tide logout` la supprime : à ne pas lancer sur une machine dont un agent dépend.

### Quand on ne peut pas se connecter là où l'agent tourne

Un agent dans un conteneur ou hébergé ailleurs n'a pas de shell pour `tide login`, ni de home
persistant pour le fichier. Il prend alors le jeton dans **`TIDE_SESSION`** et le serveur dans
**`TIDE_URL`**, lues avant le fichier (le fichier reste le cas normal). Le jeton se demande là où l'on peut se connecter :

```sh
tide login https://notes.example.ts.net --print-token
```

Il crée une session **à part**, l'affiche et n'écrit rien : celle de la machine n'est pas touchée.
Le formulaire s'affiche sur stderr, stdout ne porte que le jeton : `TIDE_SESSION=$(tide login <url>
--print-token)` le range directement dans une variable, sans qu'il passe par l'écran. Le
jeton n'est lié à aucune machine (la base n'en garde que le hash), il se donne donc à l'agent par son
environnement :

```sh
TIDE_URL=https://notes.example.ts.net TIDE_SESSION='<le-jeton>' tide mcp
```

- Elle vit comme les autres : **30 jours glissants**, relancée à chaque usage.
- Pour la couper : `TIDE_SESSION='<le-jeton>' tide logout`. Seule cette session part, le fichier
  de la machine reste.

**Ce jeton ouvre le compte entier.** Il se traite comme un mot de passe : il ne va **pas** dans un
`.mcp.json`, un `config.toml` ou tout fichier versionné, ni dans le bloc `env` d'un client dont la
configuration se partage. Il se donne par le gestionnaire de secrets de la plateforme qui héberge
l'agent (secret de conteneur, variable chiffrée du CI), ou par l'environnement du shell qui lance le
client.

### Ce que ça implique

Tout agent capable de lancer `tide mcp` sur cette machine a **tous les droits du compte connecté**,
écriture et suppression comprises. La garantie est celle de l'utilisateur Unix, exactement comme pour
`tide show note`. Il n'y a ni compte réservé aux agents, ni droits en lecture seule, ni partage
entre comptes : c'est le compte, en entier. La lecture seule existe pour le MCP en HTTP seulement, avec
un jeton d'API ou une connexion OAuth en lecture seule (section 4).

## 3. Ce qu'il peut faire

À la connexion, l'agent reçoit les trente et un outils avec leur description et leur schéma, puis travaille
en langage naturel : « range mes notes de réunion de novembre dans /Travail/Réunions » devient un
`search_notes`, un `create_folder`, puis une suite de `move_item`.

Le serveur lui donne aussi des consignes, que Claude Code met dans son prompt système : la façon économe de
traiter beaucoup de notes, celle du banc d'essai #198. Appeler `tags` une fois et garder la liste, trouver
les candidates sur le serveur avant de lire (`search_notes` avec `untagged`, `max_length`, `updated_before`),
lire par lots en aperçu (`read_notes` avec `max_chars`), découper un gros dossier en pages de `list_folder`,
une par sous-agent, taguer avec `tag_notes` (jamais en réécrivant la note par `edit_note`), un appel par tag
posé sur toutes les notes d'un lot plutôt qu'un appel par note (au passage 3 de #198, 198 appels note par note
faisaient l'essentiel du coût), fusionner deux
tags avec `rename_tag` et `merge`. Avec un jeton en lecture seule, les consignes d'écriture sont retirées.
Les sous-agents de Claude Code ne les reçoivent pas (vérifié le 2026-11-17) : les consignes demandent à
l'agent principal de leur passer ce qu'il leur faut.

| Outil | Ce qu'il fait |
|---|---|
| `search_notes` | Les notes dont le titre ou le contenu contient un texte, avec la ligne trouvée, et leur total : les 50 premières, ou `limit` (0 pour toutes) et `offset`. `max_length` (au plus N caractères, ligne de titre comprise : les notes vides ou réduites au titre) `untagged` (sans tag) et `updated_before` (modifiées avant un jour ou un instant, les plus anciennes d'abord sans texte) ne gardent que les candidates, avec ou sans texte. Un motif d'inachèvement (`[Table could not be rendered]`, un `/` seul) se cherche comme texte |
| `recent_notes` | Les notes modifiées le plus récemment, la plus récente d'abord |
| `read_note` | Le contenu d'une note, son chemin, ses tags (`[]` sans tag) et ses dates (`created_at`, `updated_at`) ; `max_chars` n'en rend que le début, avec `length` (la taille entière, en caractères) et `truncated` |
| `read_notes` | Plusieurs notes d'un coup, 50 au plus, chacune comme `read_note` ; une seule introuvable fait échouer l'appel. Pour trier beaucoup de notes, les lire d'abord en aperçu (`max_chars`), puis en entier seulement celles qui le demandent |
| `list_folder` | Les notes et dossiers directement dans un dossier : les 50 premiers et le total, ou `limit` (0 pour tous) et `offset` ; `dates` ajoute leurs dates (`created_at`, `updated_at`) |
| `tags` | Les notes portant un tag, avec leur chemin : les 50 premières et le total, ou `limit` et `offset`, `dates` ajoutant leurs dates ; sans tag, les tags du compte, les plus utilisés d'abord, `prefix` (début du nom, sans casse) et `min_notes` (au moins N notes) n'en gardant qu'une partie |
| `create_note` | Une note à partir de markdown : le premier titre devient le titre, les `#hashtags` des tags |
| `edit_note` | Remplace **tout** le contenu d'une note (son premier titre devient le titre) et la rend sans son contenu ; refuse une note verrouillée |
| `tag_notes` | Ajoute ou retire des tags sur des notes sans envoyer leur contenu : seuls leurs `#tags` changent, chaque note changée garde son texte dans l'historique ; une note verrouillée est laissée, avec l'erreur. À la place de `notes`, `text`, `max_length`, `untagged` et `updated_before` taguent toutes les notes que `search_notes` trouverait, sans les lire ; `dry_run` liste seulement ces notes |
| `create_folder` | Un dossier, à la racine ou dans un autre |
| `rename_folder` | Le titre d'un dossier |
| `move_item` | Déplace une note ou un dossier ; un dossier emmène son contenu |
| `delete_note` | Met une note à la corbeille |
| `delete_folder` | Met un dossier **et tout son contenu** à la corbeille |
| `list_trash` | Ce qui est à la corbeille, avec l'uuid que `restore_item` attend |
| `restore_item` | Ressort de la corbeille, avec ce qui y est parti en même temps |
| `rename_tag` | Renomme un tag sur toutes ses notes, dans leur texte aussi ; vers un tag qui existe déjà, refuse, sauf avec `merge` qui les fusionne |
| `replace_notes` | Remplace un texte brut dans plusieurs notes d'un coup (`folder` pour limiter, `dry_run` pour compter avant) ; les notes verrouillées sont laissées, chaque note changée garde son texte dans l'historique |
| `delete_tag` | Retire un tag de toutes ses notes ; une note qui ne contenait que lui part à la corbeille |
| `note_history` | Les versions antérieures d'une note (sans `version`), ou le texte de l'une d'elles |
| `note_links` | Les liens d'une note, ses rétroliens et les notes qui partagent ses tags ; sans note, les liens cassés de tout le compte |
| `read_file` | Une image (que l'agent voit) ou une pièce jointe citée par une note ; sans fichier, la liste des fichiers |
| `attach_file` | Stocke un fichier envoyé en base64 et rend le markdown qui le cite, à poser avec `create_note` ou `edit_note` |
| `export_notes` | Un dossier, des éléments ou tout le compte en fichiers markdown, `[[liens]]` réécrits, chaque note avec son id et sa dernière modification : les 50 premiers fichiers et le total, ou `limit` et `offset` |
| `import_notes` | Des fichiers markdown en notes, leurs répertoires en dossiers, tout ou rien ; les `[[liens]]` et les liens `[texte](chemin.md)` vers une note de l'import deviennent des liens de notes (pas ceux des notes importées avant la v1.2.0 : les réimporter ou les corriger) |
| `snippets` | Les snippets du compte, nom et contenu |
| `save_snippet` | Crée un snippet, ou remplace celui du même nom, les autres gardés |
| `delete_snippet` | Retire un snippet par son nom, les autres gardés |
| `saved_searches` | Les recherches enregistrées (dossiers intelligents) du compte, tags par nom et dossiers par chemin ; avec `search` (id, ou nom s'il est unique), les notes qu'elle trouve avec leur chemin : les 50 premières et le total, ou `limit` et `offset`. Plan Pro |
| `save_search` | Crée une recherche enregistrée (tags par nom, `tag_mode` `any` ou `all`, dossiers par chemin ou uuid, `folder_mode` `include` ou `exclude`, dates de création et de modification, texte), ou remplace celle que `search` désigne, les autres gardées. Plan Pro |
| `delete_search` | Retire une recherche enregistrée par son id ou son nom, ses notes restent. Plan Pro |

**Un outil par domaine, pas un par processus.** Chaque outil annoncé coûte du contexte à l'agent à
chaque requête : les trente et un font environ 17 Ko de nom, description et schéma d'entrée (de 450 octets
pour `read_file` à 1,6 Ko pour `save_search`, ~5 000 tokens en tout, presque toujours lus depuis le cache) ;
leurs schémas de sortie (11 Ko), les clients ne les donnent pas au modèle. Regrouper les tags, les snippets
et les recherches enregistrées en un outil de lecture et un d'écriture chacun n'en retirerait qu'environ
2 % (#208, mesuré le 2026-11-17) : le coût d'un passage est dans ce que les outils lisent et rendent, pas
dans leur liste. Lister et
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
  résoudre. Un chemin qui désigne deux éléments est refusé (`several items at this path, use a uuid`),
  avec l'uuid, le type et la date de dernière modification de chacun. Le chemin d'un export ne se relit
  pas toujours (un titre en double y devient « Titre (2) », un « / » un « - ») : l'id qu'il donne, si.
- **`edit_note` écrase.** Rien n'est conservé de l'ancien contenu, l'agent doit lire la note avant. Le
  texte remplacé va dans l'historique de la note comme pour une modification dans l'app (à la première
  modification d'une session d'écriture, puis au moins une fois par heure), d'où `note_history` et
  l'app le ressortent. Les `#tags` du contenu restent, ceux de `tags` s'y ajoutent. Pour seulement
  taguer, `tag_notes` évite de recopier la note (et d'en perdre un bout, le titre compris).
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
  comprise. `export_notes` ne télécharge pas les images citées : il n'en donne que le chemin.
- **`delete_tag` et `rename_tag` réécrivent le texte des notes**, corbeille comprise, comme la CLI et
  l'app : chaque note changée garde son texte d'avant dans l'historique.
- **`save_snippet` et `delete_snippet` relisent la liste puis la réécrivent en entier**, `save_search` et
  `delete_search` aussi, avec le `updated_at` du compte lu : une écriture arrivée entre-temps n'est pas
  effacée, l'outil relit et refait son changement une fois, puis répond l'erreur. L'app, elle, réécrit
  encore ses listes sans ce contrôle : une page de l'app restée ouverte peut effacer le changement d'un agent.
- **Les recherches enregistrées sont du plan Pro** : hors Pro, les trois outils répondent
  `saved searches (smart folders) need the Pro plan, which this account does not have`. Une recherche
  n'a qu'un `tag_mode` pour tous ses tags : « Recette ET (Végétarien OU Vegan) » ne se dit pas, il faut
  deux recherches. `save_search` remplace une recherche en entier : un critère non redonné est retiré.
  Une date seule (`2026-01-31`) vaut le début du jour en `*_from`, sa fin en `*_to`, en UTC comme l'app.
- **Ce qui n'est pas exposé** : le graphe complet (`note_links` en donne le voisinage note par note),
  les plans, la gestion des comptes, la création d'un tag seul, le vidage de la corbeille.

### Ce que le client demande avant d'agir

Chaque outil porte des annotations MCP, pour que le client demande confirmation au bon moment
(« autoriser une fois / toujours / refuser ») : c'est lui qui la propose, pas le serveur.

- `readOnlyHint` : les lectures (`search_notes`, `recent_notes`, `read_note`, `read_notes`, `list_folder`, `tags`,
  `list_trash`, `note_history`, `note_links`, `read_file`, `export_notes`, `snippets`, `saved_searches`), qui ne changent rien.
- `destructiveHint` : ce qui supprime ou écrase (`edit_note`, `replace_notes`, `delete_note`,
  `delete_folder`, `delete_tag`, `save_snippet`, `delete_snippet`, `save_search`, `delete_search`). Les autres écritures l'ont à `false` : elles ajoutent, déplacent ou renomment.
- `idempotentHint` : un second appel identique ne change rien de plus (`edit_note`, les suppressions,
  `move_item`, les renommages, `tag_notes`, `restore_item`, `save_snippet`) ; pas `create_note`, `create_folder`, `replace_notes`,
  `attach_file`, `import_notes` ni `save_search` (qui crée une nouvelle recherche à chaque appel sans `search`).
- `openWorldHint` à `false` partout : les outils ne touchent qu'aux notes du compte.

## 4. Depuis le téléphone ou ChatGPT : le MCP en HTTP

L'app Claude (téléphone, web, Desktop) et ChatGPT n'appellent pas le serveur MCP depuis l'appareil mais
**depuis leur cloud** : il doit être joignable depuis Internet, Tailscale seul ne suffit pas. Pour ça,
`tide-server serve --mcp-addr <adresse>` sert les mêmes outils en HTTP (transport « streamable HTTP »),
sur une **adresse à part** qui n'a que `/mcp` et la connexion OAuth qui va avec (section suivante).
C'est cette adresse, et elle seule, qu'on ouvre à Internet : l'API et l'app restent derrière Tailscale
ou le proxy habituel.

`compose.prod.yaml` l'active sur le port 8081 du conteneur, publié sur `TIDE_MCP_LISTEN`
(`127.0.0.1:8081` par défaut, donc fermé tant qu'aucun tunnel ne pointe dessus). `--mcp-published`
reçoit cette même adresse : sur le loopback, seul un tunnel local y arrive, et le serveur lit l'adresse
du client dans `X-Forwarded-For` (le délai après les mauvais mots de passe ne bloque que lui).
Si 8081 est déjà pris sur la machine (`address already in use` au démarrage, et le conteneur reste
arrêté), un autre port dans `.env`, `TIDE_MCP_LISTEN=127.0.0.1:8083`, et le tunnel pointe dessus.

### Deux façons de s'authentifier

Chaque requête sur `/mcp` porte `Authorization: Bearer notes_…`. Ce jeton vient de l'une de deux sources :

- **Un jeton d'API**, créé par `tide add token "Claude téléphone" --expires 30d` ou dans Réglages >
  Compte de l'app (montré une seule fois), puis collé dans le client. Pour un client qui accepte un
  en-tête fixe.
- **Une connexion OAuth** : le client ne demande que l'URL, ouvre une page de connexion de Tide, et
  reçoit ses jetons tout seul. Pour ChatGPT, qui n'accepte pas d'en-tête fixe, et pour Claude par
  « Connect ».

Sans jeton, avec un jeton inconnu, expiré ou révoqué, ou avec un jeton de **session**, la réponse est
401 avant tout outil, avec `WWW-Authenticate: Bearer resource_metadata="…"` qui dit au client où se
connecter. Un jeton d'API ou une connexion se révoque à part (`tide remove token <uuid>`, ou Réglages >
Compte) et reste interdit sur les routes du compte (jetons, mot de passe, réglages). L'agent a sinon
tous les droits du compte sur ses notes, comme `tide mcp`, **sauf en lecture seule** : un jeton créé
avec `--read-only` (la case « Lecture seule » de l'app), ou une connexion dont la case « Lecture seule »
a été cochée, ne reçoit que les outils de lecture (ceux à `readOnlyHint`), et l'API lui refuse toute
modification (403). Le serveur ne garde aucune session MCP entre deux requêtes, et ne journalise pas
les jetons.

### La connexion OAuth

Tide est son propre serveur d'autorisation OAuth 2.1, comme le décrit la spécification MCP, sur la
même adresse que `/mcp` :

| Route | Rôle |
|---|---|
| `/.well-known/oauth-protected-resource` | Dit que `/mcp` se connecte auprès de cette adresse (RFC 9728) |
| `/.well-known/oauth-authorization-server` | Les routes ci-dessous et ce qu'elles acceptent (RFC 8414) |
| `/register` | Enregistrement dynamique d'un client (RFC 7591), ouvert à tous |
| `/authorize` | La page de connexion et de consentement |
| `/token` | Échange le code contre des jetons, puis les renouvelle |

Le déroulé, côté utilisateur : on donne l'URL `…/mcp` au client, il ouvre la page de Tide, on y tape le
nom du compte et son mot de passe, on coche « Lecture seule » si on veut, puis « Autoriser ». La page
affiche le nom que le client s'est donné et l'**hôte** où le code repart (`chatgpt.com`, `claude.ai`,
`localhost:…`) : c'est cet hôte qui dit à qui l'accès est donné, le nom peut être n'importe quoi.

- **N'importe quel client** peut s'enregistrer : une URL de retour en `https`, ou en `http` sur
  `localhost` ou `127.0.0.1` (un programme de la machine, comme Claude Code ; le port peut changer à
  chaque connexion). Le client est public : PKCE (`S256`) remplace son secret, un secret envoyé quand
  même est ignoré. Le paramètre `iss` (RFC 9207) est renvoyé, ce qui permet à ChatGPT d'utiliser son
  URL de retour fixe.
- **Les jetons** : un jeton d'accès d'une heure (un jeton d'API, `notes_…`), et un jeton de
  renouvellement de 90 jours, remplacé à chaque usage, qui repart pour 90 jours. Un client utilisé au
  moins une fois tous les trois mois reste donc connecté.
- **La liste** : chaque connexion est une ligne de Réglages > Compte et de `tide show token`, au nom du
  client, marquée OAuth, avec la fin de sa connexion. La révoquer coupe les deux jetons : le client
  doit refaire la connexion.
- **Les mauvais mots de passe** de la page comptent avec ceux de la connexion à l'app, par adresse, en
  base : 30 s d'attente au quatrième d'affilée, puis de plus en plus, jusqu'à une heure, même après un
  redémarrage. La page ne s'affiche pas dans un cadre d'un autre site.
- Rien n'est gardé d'un client enregistré : son identifiant porte son nom et ses URL de retour. Un
  enregistrement ouvert ne remplit donc pas la base.

### L'ouvrir à Internet

Le serveur parle HTTP simple : le tunnel ou le proxy devant fournit le HTTPS. Il doit transmettre le
`Host` reçu : les adresses de la connexion OAuth en sont faites. Trois façons, au choix :

- **Tailscale Funnel**, sur 443 : `tailscale funnel http://127.0.0.1:8081`, puis l'URL
  `https://<machine>.<tailnet>.ts.net/mcp`. Claude ne joint pas un autre port (8443 essayé : « Impossible
  de joindre »), donc l'app doit être servie sur un autre port de la machine (`tailscale serve --https=8282`).
  Sans `--bg`, le funnel reste au premier plan et `Ctrl+C` le referme : de quoi ouvrir le temps d'un essai.
- **Cloudflare Tunnel** : un tunnel dont le service est `http://127.0.0.1:8081`
  (`cloudflared tunnel --url http://127.0.0.1:8081` pour un essai), puis `https://<nom>/mcp`.
- **Un proxy HTTPS** déjà exposé (Caddy, nginx) : les routes `/mcp`, `/.well-known/`, `/register`,
  `/authorize` et `/token` vers `127.0.0.1:8081`, rien d'autre. Le proxy peut limiter `/mcp`,
  `/register` et `/token` aux IP des clients (Anthropic et OpenAI les publient), mais pas `/authorize` :
  c'est le navigateur de l'utilisateur qui l'ouvre.

### Le déclarer dans Claude

Réglages > Connecteurs > « Add custom connector » : l'URL ci-dessus (finie par `/mcp`), rien d'autre,
puis « Connect » : la page de Tide s'ouvre. Claude revient sur `https://claude.ai/api/mcp/auth_callback`.
Le connecteur apparaît ensuite sur le téléphone avec le même compte Claude.

Avec un jeton d'API à la place : dans « Request headers », l'en-tête `Authorization` avec la valeur
`Bearer notes_…`. Anthropic réserve encore ces en-têtes à certaines organisations (bêta) : si la
section n'apparaît pas, c'est OAuth.

### Le déclarer dans ChatGPT

ChatGPT ne propose qu'OAuth ou rien. Réglages > Apps > Avancé > « Developer mode », puis créer une app
(ou connecteur) avec l'URL `…/mcp` et l'authentification OAuth, sans identifiant ni secret de client :
ChatGPT s'enregistre seul, ouvre la page de Tide, et revient sur
`https://chatgpt.com/connector_platform_oauth_redirect`. ChatGPT demande confirmation avant chaque
outil qui écrit (ceux sans `readOnlyHint`). OpenAI ne dit pas clairement quels abonnements ont le
Developer mode et l'écriture (Plus et Pro seraient limités selon certaines sources) : l'essayer sur le
compte.

### N'importe quel autre client

Trois façons, de la plus simple à la plus générale :

1. **Le client lance une commande** (Claude Code, Codex, Cursor, un agent en local) : `tide mcp` en
   stdio, sections 1 et 2. Rien à ouvrir à Internet.
2. **Le client accepte un en-tête** : l'URL `…/mcp` et `Authorization: Bearer notes_…`, un jeton
   d'API (en lecture seule si l'agent n'a qu'à lire).
3. **Le client sait faire OAuth** (MCP « remote » avec connexion) : l'URL `…/mcp` seule. Il trouve le
   reste par le 401 et les `/.well-known`, s'enregistre, ouvre la page de Tide dans le navigateur,
   et renouvelle ses jetons tout seul.

Pour vérifier depuis un terminal, sans client :

```sh
curl -s https://<adresse>/mcp -H 'Authorization: Bearer notes_…' \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
curl -s https://<adresse>/.well-known/oauth-authorization-server
```

La liste des outils en JSON : il marche. `401` : le jeton (inconnu, expiré, révoqué, ou une session).
Les métadonnées OAuth en JSON, avec l'adresse `https://` publique : la connexion OAuth est servie.
Rien : le tunnel ou `--mcp-addr`.

## Vérifier que ça marche

Le test `cli/cmd-mcp/cmd_test.go` branche un client MCP en mémoire sur le serveur MCP, lui-même
client d'une vraie API sur la base de test. Il vérifie que les trente et un outils sont annoncés avec un
schéma, une description et leurs annotations — ce que le SDK ne sait pas déduire seul d'une structure d'arguments mal
formée — puis les appelle un à un.

`core/server/mcphttp/mcphttp_test.go` sert les outils en HTTP et vérifie le 401 sans jeton, avec une session, un jeton
inconnu, expiré ou révoqué, puis un aller-retour avec un jeton valide, et les seuls outils de lecture
pour un jeton en lecture seule. `core/server/oauth/oauth_test.go` fait le parcours OAuth d'un client :
métadonnées, enregistrement, page de consentement, échange du code, renouvellement.

```sh
make test ARGS="./cli/cmd-mcp/ ./core/server/mcphttp/ ./core/server/oauth/"
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
  sleep 3; } | tide mcp
```

Le `sleep` garde l'entrée ouverte le temps des réponses : sans lui, le serveur s'arrête avant d'avoir
répondu.

Quand il échoue, dans l'ordre :

1. **La session** : `tide whoami` sous l'utilisateur Unix qui lance l'agent dit pour quel compte
   et quel serveur il agit. Rien : `tide login <url>`, ou `TIDE_URL` et `TIDE_SESSION`.
2. **Le client lui donne-t-il un bloc `env`** avec un `TIDE_URL` ou un `TIDE_SESSION` ? Ils gagnent
   sur le fichier : une valeur périmée masque une session valide (voir le piège, section 1).
3. **Le serveur est-il joignable** depuis la machine de l'agent (Tailscale allumé, URL juste) ?
