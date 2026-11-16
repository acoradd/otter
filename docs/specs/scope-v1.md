# Otter — Spécification

Client HTTP desktop léger, alternative à Postman/Bruno, pensé pour de grosses collections : chargement paresseux de l'arborescence, recherche plein texte instantanée, résolution des variables et headers visible avant l'envoi.

Ce document vit dans `docs/specs/scope-v1.md`. Les règles d'architecture et les conventions sont dans `CLAUDE.md` à la racine. Lis les deux en entier avant de commencer. Avance étape par étape (voir « Plan de réalisation »), avec des tests à chaque étape. En cas d'ambiguïté non couverte ici, pose la question plutôt que d'inventer.

---

## 1. Stack

| Couche | Choix |
|---|---|
| Desktop | Wails v3, version exacte épinglée dans `src/go.mod` (même version que la CLI `wails3` et `@wailsio/runtime`) |
| Backend | Go (dernière version stable) |
| Base de données | SQLite via `modernc.org/sqlite` (pur Go, sans CGO), FTS5 |
| Moteur de script | `github.com/dop251/goja` |
| Secrets | `github.com/zalando/go-keyring` + AES-GCM |
| Front | Angular (dernière version stable), composants standalone, signals |
| Composants UI | Angular CDK `ScrollingModule` (arbre virtualisé). Glisser-déposer maison en pointer events (pas de HTML5 drag and drop, peu fiable entre WebView2, WebKitGTK et WKWebView) |
| i18n | Transloco, changement de langue à chaud, `en` et `fr` |
| Éditeur | Monaco (body, réponse, script), chargé directement via `monaco-editor` dans un composant Angular dédié, sans wrapper tiers non maintenu |

Contraintes :

- Aucune dépendance CGO.
- Migrations SQL embarquées (`embed`), appliquées au démarrage, versionnées (`PRAGMA user_version` ou table `schema_migrations`).
- `PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON` à l'ouverture.
- Base unique `otter.db` dans le dossier de config utilisateur de l'OS (`os.UserConfigDir()/Otter`), reconnue par son `application_id` SQLite (`OTTR`), versionnée par `user_version`. Ouverture avec un chemin simple, jamais une URI `file:`.
- Intégration Angular dans Wails 3 : utiliser le template Angular officiel s'il existe pour la version de Wails utilisée. Sinon, partir du template TypeScript de base, remplacer le dossier `frontend/` par un projet Angular CLI, et configurer les commandes dev/build et le dossier de sortie (`dist/.../browser`) dans la config Wails. Vérifier que le hot reload fonctionne en mode `wails3 dev`.
- Front : `ChangeDetectionStrategy.OnPush` partout, état géré par signals (pas de NgRx).

---

## 2. Périmètre

### Inclus

- Workspaces, chacun avec une arborescence de dossiers imbriqués et des environnements
- Variables et headers au niveau dossier, variables et headers au niveau requête
- Requêtes HTTP : méthode, URL, headers, body (aucun, JSON, XML, texte, form URL-encoded, multipart avec fichiers, fichier binaire), script post-request, paramètres (encodage URL, suivi des redirections, timeout)
- Interpolation `{{var}}` dans l'URL, les headers (nom et valeur) et le body
- Résolution visible avant l'envoi : listes Headers et Variables fusionnées, survol des variables, aperçu de la requête finale
- Recherche plein texte
- Historique des envois
- Import/export : OpenAPI (URL ou fichier), Bruno, Postman, Otter, cURL. Export Otter, OpenAPI, Bruno, Postman
- Coller une commande cURL dans la barre d'URL pour créer la requête
- Génération de code de la requête finale (cURL, PowerShell, fetch, Python, Go, Java, C#, PHP, HTTP brut)

### Hors périmètre (v1)

- Gestion Git
- Collection runner
- Scripts pre-request
- Flows d'auth dédiés (OAuth2, Digest, NTLM) : l'auth passe par des headers et des variables
- GraphQL, WebSocket, gRPC, SSE
- Synchronisation cloud, collaboration

---

## 3. Modèle de données

```sql
CREATE TABLE workspace (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  active_environment_id INTEGER REFERENCES environment(id) ON DELETE SET NULL,
  -- Réglages réseau du workspace
  proxy_url TEXT NOT NULL DEFAULT '',          -- vide = proxy système (variables d'env)
  tls_insecure INTEGER NOT NULL DEFAULT 0,
  ca_cert_path TEXT NOT NULL DEFAULT '',
  client_cert_path TEXT NOT NULL DEFAULT '',
  client_key_path TEXT NOT NULL DEFAULT '',
  cookie_jar INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE environment (
  id INTEGER PRIMARY KEY,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE env_var (
  id INTEGER PRIMARY KEY,
  environment_id INTEGER NOT NULL REFERENCES environment(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  key TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',              -- chiffrée si secret = 1
  secret INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE folder (
  id INTEGER PRIMARY KEY,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  parent_id INTEGER REFERENCES folder(id) ON DELETE CASCADE, -- NULL = racine du workspace
  name TEXT NOT NULL,
  seq INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_folder_children ON folder(workspace_id, parent_id, seq);

CREATE TABLE folder_var (
  id INTEGER PRIMARY KEY,
  folder_id INTEGER NOT NULL REFERENCES folder(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  key TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE folder_header (
  id INTEGER PRIMARY KEY,
  folder_id INTEGER NOT NULL REFERENCES folder(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  key TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'set' CHECK (mode IN ('set','ignore')),
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE request (
  id INTEGER PRIMARY KEY,
  folder_id INTEGER NOT NULL REFERENCES folder(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  name TEXT NOT NULL,
  method TEXT NOT NULL DEFAULT 'GET',
  url TEXT NOT NULL DEFAULT '',
  body_type TEXT NOT NULL DEFAULT 'none'
    CHECK (body_type IN ('none','json','xml','text','form-urlencoded','multipart','binary')),
  body TEXT NOT NULL DEFAULT '',                 -- json, xml, text
  body_file TEXT NOT NULL DEFAULT '',            -- binary : chemin du fichier
  post_script TEXT NOT NULL DEFAULT '',
  encode_url INTEGER NOT NULL DEFAULT 1,
  follow_redirects INTEGER NOT NULL DEFAULT 1,
  max_redirects INTEGER NOT NULL DEFAULT 10,
  timeout_ms INTEGER NOT NULL DEFAULT 30000
);
CREATE INDEX idx_request_folder ON request(folder_id, seq);

CREATE TABLE request_var (
  id INTEGER PRIMARY KEY,
  request_id INTEGER NOT NULL REFERENCES request(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  key TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE request_header (
  id INTEGER PRIMARY KEY,
  request_id INTEGER NOT NULL REFERENCES request(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  key TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  mode TEXT NOT NULL DEFAULT 'set' CHECK (mode IN ('set','ignore')),
  enabled INTEGER NOT NULL DEFAULT 1
);

-- Champs d'un body form-urlencoded ou multipart
CREATE TABLE request_body_field (
  id INTEGER PRIMARY KEY,
  request_id INTEGER NOT NULL REFERENCES request(id) ON DELETE CASCADE,
  seq INTEGER NOT NULL DEFAULT 0,
  key TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'text' CHECK (kind IN ('text','file')), -- 'file' : multipart uniquement
  value TEXT NOT NULL DEFAULT '',              -- texte, ou chemin du fichier
  content_type TEXT NOT NULL DEFAULT '',       -- optionnel, par partie (multipart)
  enabled INTEGER NOT NULL DEFAULT 1
);

-- Variables posées par les scripts post-request, persistées entre sessions
CREATE TABLE runtime_var (
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  environment_id INTEGER NOT NULL DEFAULT 0,   -- 0 = aucun environnement actif
  key TEXT NOT NULL,
  value TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, environment_id, key)
);

-- Source d'un import resynchronisable (OpenAPI par URL)
CREATE TABLE import_source (
  id INTEGER PRIMARY KEY,
  workspace_id INTEGER NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
  folder_id INTEGER REFERENCES folder(id) ON DELETE CASCADE, -- dossier créé par l'import
  kind TEXT NOT NULL CHECK (kind IN ('openapi')),
  url TEXT NOT NULL,
  last_sync_at INTEGER,                         -- epoch ms
  last_hash TEXT NOT NULL DEFAULT ''
);

CREATE TABLE history (
  id INTEGER PRIMARY KEY,
  request_id INTEGER REFERENCES request(id) ON DELETE SET NULL,
  workspace_id INTEGER REFERENCES workspace(id) ON DELETE CASCADE,
  sent_at INTEGER NOT NULL,                    -- epoch ms
  method TEXT, url TEXT, status INTEGER, duration_ms INTEGER, size_bytes INTEGER,
  req_headers TEXT, req_body TEXT, res_headers TEXT, res_body TEXT,
  error TEXT
);
CREATE INDEX idx_history_request ON history(request_id, sent_at DESC);

-- Recherche par mots, accents ignorés
CREATE VIRTUAL TABLE request_fts USING fts5(
  name, url, body,
  content='request', content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
-- Recherche par sous-chaîne (URLs, identifiants)
CREATE VIRTUAL TABLE request_fts_tri USING fts5(
  name, url,
  content='request', content_rowid='id',
  tokenize='trigram'
);
-- Triggers AFTER INSERT / UPDATE / DELETE sur request pour tenir les deux index à jour
-- (syntaxe 'delete' des tables FTS5 à contenu externe).
```

Règles d'intégrité applicatives :

- Déplacer un dossier : interdire de le placer dans un de ses descendants (vérification par CTE récursive).
- Réordonner : renuméroter `seq` des éléments frères dans une transaction.
- Un dossier et une requête ne changent jamais de workspace.

---

## 4. Résolution des variables

### Sources, de la plus prioritaire à la moins prioritaire

1. Variables runtime (workspace + environnement actif)
2. Variables de la requête
3. Variables du dossier de la requête, puis de ses parents jusqu'à la racine (le plus proche gagne)
4. Environnement actif du workspace

Seules les lignes `enabled = 1` participent. Les variables ne peuvent pas être « ignorées », seulement redéfinies à un niveau plus proche.

### Interpolation

- Syntaxe : `{{nom}}`, espaces tolérés (`{{ nom }}`), nom en `[\w.\-]+`.
- Appliquée à : URL, nom et valeur de chaque header, body texte (JSON, XML, texte), nom et valeur des champs form et multipart, chemin des fichiers. Jamais au contenu d'un fichier envoyé.
- Récursive (une valeur peut contenir `{{autre}}`), profondeur maximale 10, détection de cycle signalée en avertissement.
- Variable introuvable : le placeholder reste tel quel et la variable est listée dans `Missing`.

### Chaîne des dossiers

Une seule requête SQL par type (vars, headers) :

```sql
WITH RECURSIVE chain(id, parent_id, depth) AS (
  SELECT id, parent_id, 0 FROM folder WHERE id = :folder_id
  UNION ALL
  SELECT f.id, f.parent_id, c.depth + 1 FROM folder f JOIN chain c ON f.id = c.parent_id
)
SELECT c.depth, c.id, x.key, x.value
FROM chain c JOIN folder_var x ON x.folder_id = c.id AND x.enabled = 1
ORDER BY c.depth DESC, x.seq;
```

---

## 5. Résolution des headers

Traitement par couches, de la racine vers la requête :

1. Dossiers, de la racine au dossier de la requête
2. Headers automatiques
3. Requête

Pour chaque ligne `enabled = 1`, clé comparée sans tenir compte de la casse :

- `mode = 'set'` : ajoute le header ou remplace la valeur héritée (l'ancienne valeur passe dans `Shadowed`). Une redéfinition annule un `ignore` antérieur.
- `mode = 'ignore'` : retire le header hérité. Il reste visible dans la prévisualisation, marqué ignoré, avec `IgnoredBy` (couche qui l'a ignoré). La valeur de la ligne est sans effet.

Un dossier qui ignore un header le retire pour tout son sous-arbre. Un sous-dossier ou la requête peut le redéfinir.

### Headers automatiques

Ajoutés seulement s'ils ne sont pas déjà définis par une couche dossier. La requête peut les ignorer ou les redéfinir.

| Header | Valeur |
|---|---|
| `Content-Type` | Déduit de `body_type` : `application/json`, `application/xml`, `text/plain`, `application/x-www-form-urlencoded`, `multipart/form-data; boundary=…` (boundary générée à l'envoi, affichée « calculée »), pour `binary` le type déduit de l'extension du fichier (sinon `application/octet-stream`). Absent si `none`. |
| `User-Agent` | User-Agent de Chrome stable, au format réduit, adapté à l'OS sur lequel tourne Otter (voir ci-dessous). Ne jamais laisser `Go-http-client/1.1` partir sans qu'il soit visible. |
| `Accept` | `*/*` |
| `Accept-Encoding` | `gzip, deflate, br` |

User-Agent Chrome par OS (format réduit de Chrome : seul le numéro de version majeure varie, les autres segments sont figés) :

```
Windows : Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/<MAJEURE>.0.0.0 Safari/537.36
macOS   : Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/<MAJEURE>.0.0.0 Safari/537.36
Linux   : Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/<MAJEURE>.0.0.0 Safari/537.36
```

- `<MAJEURE>` est une constante unique dans le code (`ChromeMajorVersion`), renseignée avec la version stable de Chrome au moment du développement, facile à mettre à jour.
- L'OS est choisi via `runtime.GOOS`.
- Un dossier ou une requête peut redéfinir ou ignorer le header comme n'importe quel autre.

Comme `Accept-Encoding` est posé explicitement, le transport Go ne décompresse plus automatiquement : décompresser soi-même gzip, deflate et brotli (`github.com/andybalholm/brotli`). Afficher la taille compressée et décompressée.

`Host` et `Content-Length` sont affichés en lecture seule dans la prévisualisation (calculés), non modifiables.

---

## 6. Prévisualisation

La même fonction produit la prévisualisation et la requête envoyée. `Send` construit la requête HTTP uniquement à partir du résultat de `Preview`. C'est une exigence forte : ce qui est affiché est exactement ce qui part.

```go
type SourceKind string // "runtime" | "request" | "folder" | "env" | "auto"

type Source struct {
	Kind  SourceKind `json:"kind"`
	ID    int64      `json:"id,omitempty"`
	Label string     `json:"label"`           // ex. "OGP > auth", "env: recette"
	Depth int        `json:"depth,omitempty"` // 0 = dossier de la requête
}

type Candidate struct {
	Value  string `json:"value"`
	Source Source `json:"source"`
}

type ResolvedVar struct {
	Key      string      `json:"key"`
	Value    string      `json:"value"`
	Source   *Source     `json:"source"`   // nil = non résolue
	Shadowed []Candidate `json:"shadowed"` // définitions masquées, par priorité décroissante
	Secret   bool        `json:"secret"`
	UsedIn   []string    `json:"usedIn"`   // "url", "header:Authorization", "body"
}

type ResolvedHeader struct {
	Key       string      `json:"key"`
	RawValue  string      `json:"rawValue"`
	Value     string      `json:"value"`
	Source    Source      `json:"source"`
	Shadowed  []Candidate `json:"shadowed"`
	Ignored   bool        `json:"ignored"`
	IgnoredBy *Source     `json:"ignoredBy,omitempty"`
	ReadOnly  bool        `json:"readOnly"` // Host, Content-Length
}

type Preview struct {
	Method   string           `json:"method"`
	URL      string           `json:"url"`
	Body     string           `json:"body"`
	Vars     []ResolvedVar    `json:"vars"`
	Headers  []ResolvedHeader `json:"headers"`
	Missing  []string         `json:"missing"`
	Warnings []string         `json:"warnings"` // cycle, header vide après interpolation, etc.
}
```

Avertissements à produire au minimum :

- variable non résolue
- cycle d'interpolation
- header dont la valeur est vide après interpolation (il est quand même envoyé)
- URL invalide après interpolation

Comportement UI :

Pas d'écran de résolution dédié. Le `Preview` alimente directement l'interface :

- **Onglets Headers et Variables** : listes fusionnées (section 10), qui montrent l'état résolu au niveau de la requête.
- **Variables runtime** : elles apparaissent dans l'onglet Variables de la requête avec une pastille « runtime », puisqu'elles passent devant toutes les autres sources. Effaçables depuis la ligne.
- **Variables non résolues** : `{{var}}` surligné en rouge (soulignement ondulé) dans l'URL, les headers et le body, compteur rouge sur l'onglet concerné. Elles apparaissent aussi dans l'onglet Variables, en rouge (« Non définie · utilisée dans le body »), avec un bouton « Définir ici ».
- **Survol d'un `{{var}}`**, partout (URL, valeurs de headers, body) : valeur résolue, source, et valeur masquée s'il y en a une.
- **Icône « Générer le code »** (`</>`) à côté d'« Envoyer » : panneau latéral avec la requête finale dans le langage choisi (liste déroulante, dernier choix mémorisé), les avertissements, une case « Remplacer les variables » (cochée par défaut, sinon les `{{var}}` restent dans le code) et une case « Afficher les secrets » (décochée par défaut). Boutons « Copier » et « Enregistrer dans un fichier ». Voir section 7.
- Recalcul en direct pendant l'édition (debounce ~150 ms).

---

## 7. Envoi HTTP

- Un `http.Client` construit par envoi à partir des réglages du workspace et de la requête.
- **Timeout** : `timeout_ms` de la requête.
- **Annulation** : bouton « Annuler » qui annule le `context` de l'envoi en cours.
- **Redirections** : si `follow_redirects = 0`, `CheckRedirect` renvoie `http.ErrUseLastResponse`. Sinon erreur au-delà de `max_redirects`. Conserver la chaîne de redirections et l'afficher.
- **Encodage d'URL** :
  - `encode_url = 1` : parser l'URL interpolée, ré-encoder les paramètres de query.
  - `encode_url = 0` : envoyer le chemin et la query tels quels, sans normalisation par Go (renseigner `RawPath`/`RawQuery`, ou `Opaque` si nécessaire). Couvrir par un test avec `httptest.Server` qui vérifie la requête reçue octet pour octet.
- **Body** :
  - `json`, `xml`, `text` : texte interpolé, édité dans Monaco avec la coloration du type.
  - `form-urlencoded` : tableau clé/valeur (case à cocher par ligne), encodé à l'envoi.
  - `multipart` : tableau de champs, chacun texte ou fichier, avec Content-Type optionnel par partie (déduit de l'extension pour un fichier). Glisser-déposer d'un fichier dans le tableau pour l'ajouter. Le body est streamé (`mime/multipart` + `io.Pipe`), jamais chargé entièrement en mémoire.
  - `binary` : un fichier envoyé tel quel, streamé.
  - Les fichiers sont référencés par leur chemin et lus au moment de l'envoi. Fichier introuvable : avertissement dans le `Preview` (ligne en rouge, compteur sur l'onglet Body), l'envoi reste possible sans cette partie.
- **Réseau** : proxy du workspace (ou proxy système si vide), CA additionnelle, certificat client, option TLS insecure.
- **Cookies** : un `cookiejar` en mémoire par workspace si `cookie_jar = 1`, vidé au redémarrage, consultable et vidable depuis l'UI.
- **Réponse** : statut, headers, body, durée, taille. Body affiché dans Monaco avec formatage automatique selon le content-type (JSON, XML, HTML). Au-delà de 5 Mo, ne pas charger dans Monaco : proposer d'enregistrer dans un fichier. Body binaire : proposer l'enregistrement.
- **Historique** : chaque envoi est enregistré dans `history` (secrets masqués dans les headers enregistrés, fichiers enregistrés par nom et taille, jamais leur contenu). Conserver les 100 derniers envois par requête, purge automatique.
- **Génération de code** (`internal/codegen`, un générateur par langage, tous construits à partir du `Preview`) : cURL bash, cURL Windows cmd, PowerShell (`Invoke-WebRequest`), HTTP brut, JavaScript `fetch`, Python `requests`, Go `net/http`, Java `HttpClient`, C# `HttpClient`, PHP cURL. Chaque générateur gère les sept types de body (multipart avec fichiers, fichier binaire, form URL-encoded), les redirections et le timeout quand le langage le permet (`-L`, `--max-time`), et l'échappement propre au langage. Tests golden par langage et par type de body.
- **Coller une commande cURL** : coller une commande `curl` dans la barre d'URL (d'un onglet vide ou existant) la convertit en requête. Si l'onglet contient déjà une requête, elle est remplacée, avec « Annuler » dans le toast. Parseur dans `internal/importexport/curl` : continuations de ligne bash (`\`), cmd (`^`) et PowerShell (`` ` ``), guillemets simples et doubles, `-X`, `-H`, `-d`/`--data`/`--data-raw`/`--data-binary` (`@fichier` → body Fichier), `--data-urlencode`, `-F` (multipart, `@fichier` → champ fichier), `-u` (→ header `Authorization: Basic`), `-b` (→ header `Cookie`), `-A`, `-e`, `-L`, `--max-time`, `--compressed`, `-G`. Options sans équivalent (`-k`, `--cert`…) ignorées et listées dans le toast. Le body JSON est détecté par le `Content-Type` ou par le contenu.

---

## 8. Script post-request

Exécuté dans `goja` après réception de la réponse, uniquement si `post_script` n'est pas vide.

API exposée (compatible avec les scripts Bruno courants) :

```js
res.status        // number
res.headers       // objet, plus res.getHeader(name) insensible à la casse
res.body          // objet JS si JSON, sinon string
res.time          // durée en ms

bru.setVar(key, value)     // écrit une variable runtime
bru.getVar(key)            // lit la valeur résolue (toutes sources)
bru.setEnvVar(key, value)  // alias de setVar (écrit en runtime, ne modifie pas l'environnement)
bru.getEnvVar(key)         // lit l'environnement actif

console.log(...)           // capturé dans un onglet « Console » de la réponse
```

- Timeout d'exécution : 2 s (`vm.Interrupt`).
- Les erreurs de script sont affichées dans la console, la réponse reste affichée.
- Les variables runtime sont persistées dans `runtime_var` pour le workspace et l'environnement actifs.
- Prévoir un onglet « Variables runtime » par workspace pour les voir et les supprimer.

---

## 9. Recherche

- Champ de recherche global sur le workspace actif.
- Combiner les deux index : `request_fts` (mots, accents ignorés) et `request_fts_tri` (sous-chaînes, à partir de 3 caractères).
- Résultats : nom, méthode, URL, chemin du dossier (« OGP > auth »). Un clic ouvre la requête et déplie l'arbre jusqu'à elle.
- Recherche aussi sur les noms de dossiers (requête `LIKE` suffisante).
- Objectif : moins de 50 ms sur 10 000 requêtes.

---

## 10. Interface

- Barre latérale : sélecteur de workspace, sélecteur d'environnement, recherche, arbre.
- **Arbre paresseux** : déplier un dossier charge uniquement ses enfants directs (`SELECT ... WHERE parent_id = ? ORDER BY seq`). Les enfants chargés sont gardés en cache côté front et invalidés sur modification.
- Arbre virtualisé : liste aplatie des nœuds visibles rendue avec `cdk-virtual-scroll-viewport` (hauteur de ligne fixe), pour rester fluide avec des milliers de nœuds dépliés. Ne pas utiliser `mat-tree` imbriqué, qui rend tous les nœuds.
- Glisser-déposer (pointer events) pour déplacer et réordonner dossiers et requêtes.
- Menu contextuel : nouveau dossier, nouvelle requête, renommer, dupliquer, supprimer, déplacer.
- Onglets de requêtes, avec indicateur de modification non enregistrée. Enregistrement par `Ctrl+S`.
- Onglet de requête : barre méthode + URL + icône Générer le code + Envoyer, puis onglets Headers, Variables, Body, Script, Paramètres. Les listes Headers et Variables sont fusionnées (voir ci-dessous), la vue de chaque niveau vient d'une query Go (`GetRequestView`, `GetFolderView`) qui renvoie les lignes locales et héritées avec leur source.
- Édition d'un dossier : onglets Variables et Headers.
- **Listes fusionnées** (Headers et Variables, dans une requête comme dans un dossier) : une seule liste montre tout ce qui s'applique à ce niveau. Pas de colonne source, ni de mention de ce qu'une ligne remplace : la source s'affiche au survol de la ligne (« Défini ici », « Hérité de OGP › auth », « Hérité de l'environnement recette », « Header automatique »).
  - Ligne en blanc : définie au niveau courant, éditable. Si elle redéfinit une valeur héritée, un bouton permet de revenir à la valeur héritée (supprime la ligne locale).
  - Ligne en gris : héritée d'un dossier parent, de l'environnement, ou header automatique (dans une requête uniquement). Modifier sa valeur crée une redéfinition au niveau courant, la ligne passe en blanc.
  - Case à cocher d'un header hérité : décocher crée une ligne `mode = 'ignore'` au niveau courant (ligne barrée), recocher la supprime. Case à cocher d'une ligne locale : `enabled`.
  - Les variables héritées n'ont pas de case à cocher (une variable ne s'ignore pas, elle se redéfinit).
  - Le mode `set`/`ignore` n'est jamais exposé tel quel dans l'interface.
- Édition d'un environnement : tableau clé, valeur, secret, actif.
- Raccourcis : `Ctrl+Entrée` envoyer, `Ctrl+S` enregistrer, `Ctrl+K` ou `Ctrl+P` recherche, `Ctrl+W` fermer l'onglet.
- Thèmes clair et sombre, suivant le système par défaut.

---

## 11. Secrets

- Les valeurs `env_var` avec `secret = 1` sont chiffrées en base (AES-GCM).
- La clé est générée au premier lancement et stockée dans le trousseau de l'OS (`go-keyring`).
- Si le trousseau est indisponible, le signaler clairement à l'utilisateur, ne jamais stocker la clé en clair à côté de la base.
- Les secrets sont masqués dans l'UI, l'historique et les exports (sauf option d'export explicite).

---

## 12. Import / export

Accès : menu du workspace (sélecteur en haut à gauche) et menu contextuel d'un dossier (« Importer ici », « Exporter ce dossier »). Glisser un fichier `.json`, `.yaml` ou un dossier de collection sur la fenêtre ouvre l'import pré-rempli.

### Import

Une fenêtre unique : choix de la source, destination (nouveau workspace, ou nouveau dossier dans un workspace existant), puis **analyse avant import** (nombre de requêtes, dossiers, environnements, liste de ce qui sera converti ou ignoré). Rien n'est écrit avant « Importer ». Import dans une seule transaction, requêtes préparées.

- **OpenAPI 3.0 / 3.1 et Swagger 2.0**, en YAML ou JSON, **par URL ou par fichier**. L'URL est récupérée avec les réglages réseau du workspace (proxy, CA, certificat client). Mapping :
  - tags → dossiers (opération sans tag → dossier racine de l'import, plusieurs tags → premier tag)
  - opérations → requêtes (nom : `summary`, sinon `operationId`, sinon méthode + chemin)
  - `servers` → environnements, avec une variable `baseUrl` (variables de serveur résolues avec leur valeur par défaut)
  - paramètres de chemin → `{{param}}` dans l'URL et variable de requête vide à compléter. Paramètres de query → dans l'URL, désactivés s'ils sont optionnels. Paramètres de header → headers de la requête
  - `requestBody` → type de body selon le media type (`application/json`, `application/xml`, `application/x-www-form-urlencoded`, `multipart/form-data`, `application/octet-stream`), contenu tiré de l'exemple, sinon généré à partir du schéma (`$ref` résolus)
  - `securitySchemes` : bearer → `Authorization: Bearer {{token}}` sur le dossier racine de l'import, API key → header ou query avec variable, basic → `Authorization: Basic {{basicAuth}}`, OAuth2 / OpenID → header à compléter (pas de flow intégré), signalé dans l'analyse
- **Resynchronisation OpenAPI** : si l'import vient d'une URL et que « Garder l'URL » est coché, l'URL est stockée (`import_source`). « Resynchroniser » sur le dossier importé recalcule l'analyse sous forme de différences (opérations ajoutées, modifiées, supprimées). Les ajouts sont appliqués, les modifications ne touchent jamais une requête éditée localement (signalée à la place), les suppressions sont seulement listées.
- **Bruno** (YAML / OpenCollection, Bruno v4) : un dossier de collection devient un workspace ou un dossier, avec dossiers, requêtes, headers, variables, bodies (y compris form URL-encoded, multipart et fichiers), environnements et scripts post-response. Des fichiers d'exemple seront fournis pour caler le mapping. Ne pas implémenter avant de les avoir.
- **Postman** : collection v2.1 et environnements.
- **Otter** : export natif JSON, sans perte.
- **cURL** : même parseur que le collage dans la barre d'URL, pour une ou plusieurs commandes.

### Export

Contenu : workspace entier ou un dossier. Destination : fichier ou dossier choisi par dialogue système (le dernier dossier utilisé est mémorisé).

- **Otter** : JSON, réimportable sans perte.
- **OpenAPI 3.1** (YAML ou JSON) : dossiers → tags, requêtes → opérations (requêtes de même méthode et même chemin fusionnées), `{{var}}` du chemin → paramètres de chemin, query → paramètres, headers de la requête → paramètres de header, bodies → `requestBody` avec exemple (option), environnements → `servers` (option). Perdus et signalés : scripts, variables runtime, headers ignorés, héritage des variables.
- **Bruno** : dossier de collection YAML, pour partager avec des collègues sous Bruno.
- **Postman** : collection v2.1.
- Secrets exclus par défaut dans tous les formats (case « Inclure les secrets », avec avertissement).

---

## 13. Architecture du code

Même organisation que le projet OpenShuttle. Le détail des règles est dans `CLAUDE.md`.

```
CLAUDE.md, README.md, LICENSE
docs/specs/scope-v1.md          cette spécification
docs/adr/                       décisions d'architecture
.github/workflows/              CI et release
src/go.mod                      module Go unique
src/internal/apperr/            codes d'erreur stables (Code, *Error)
src/internal/resolve/           pur : interpolation, résolution vars/headers → Preview
src/internal/httpx/             envoi à partir d'un Preview, redirections, encodage, décompression, cURL
src/internal/script/            goja, API res/bru
src/internal/store/             SQLite : ouverture, migrations append-only, requêtes, FTS
src/internal/secrets/           trousseau OS + AES-GCM
src/internal/importexport/      natif, OpenAPI, Bruno, Postman, parseur cURL
src/internal/codegen/           génération de code par langage à partir d'un Preview
src/internal/version/           version de l'appli
src/apps/desktop/               appli Wails : main.go, services liés au front, fenêtre
src/apps/desktop/frontend/      appli Angular
```

- `resolve` est pur : pas d'I/O, pas de SQLite, pas d'import Wails, pas de lecture d'horloge. Il prend les couches déjà chargées et renvoie un `Preview`. Toute la logique de priorité y est, couverte par des tests table-driven (priorités, ignore puis redéfinition, cycles, variables manquantes, casse des headers, headers automatiques).
- Les services Wails reçoivent leurs dépendances (store, `now`, `emit`, transport HTTP, dialogues) pour être testés sans Wails.
- Bindings TypeScript générés par Wails, commités, importés via l'alias `@bindings/*`.

## 14. Plan de réalisation

Chaque étape se termine par une appli qui compile, des tests qui passent et une démonstration possible.

1. **Socle** : arborescence du dépôt, projet Wails 3 + Angular + Transloco, fenêtre sans barre native, CI (tests Go et front), ouverture SQLite, migrations, codes d'erreur, CRUD workspaces et environnements.
2. **Arbre** : CRUD dossiers et requêtes, arbre paresseux virtualisé, glisser-déposer, menu contextuel. Script de génération de données de test (10 000 requêtes, 6 niveaux de profondeur) pour valider la fluidité.
3. **Édition** : onglets de requêtes, éditeur méthode/URL, tableaux headers et variables (avec `mode` pour les headers), body dans Monaco, paramètres, édition des dossiers.
4. **Résolution** : package `resolve` complet avec tests, listes fusionnées Headers et Variables, surlignage et survol des variables.
5. **Envoi** : `httpx`, réponse, annulation, redirections, encodage d'URL (tests `httptest`), décompression, tous les types de body, historique.
6. **cURL et code** : parseur cURL et collage dans la barre d'URL, `codegen` pour chaque langage, panneau Générer le code.
7. **Script post-request** : goja, API `res`/`bru`, console, variables runtime.
8. **Recherche** : index FTS, triggers, UI de recherche, objectif de performance.
9. **Réseau et secrets** : réglages proxy/TLS/cookies du workspace, chiffrement des secrets.
10. **Import/export** : fenêtres d'import et d'export avec analyse, natif, OpenAPI (URL, fichier, resynchronisation, export), Postman, puis Bruno une fois les fichiers d'exemple fournis.
