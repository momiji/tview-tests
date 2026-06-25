# Migration de `kpx/` vers `internal/`

Ce document est le **plan directeur** du portage du code historique `kpx/`
vers l'architecture en couches de ce projet. Il sert de référence pour
l'inventaire, l'architecture cible, la refonte de `process.go`, et l'ordre
des étapes de migration.

Conformément à `CLAUDE.md`, chaque item est migré en deux temps : (1) on
**propose un découpage et on s'arrête pour validation**, puis (2) une fois
validé, on **copie avec une transformation minimale**. Ce fichier couvre la
phase de planification globale ; chaque étape ci-dessous redéclenchera sa
propre micro-validation au moment de la coder.

---

## 1. Inventaire de `kpx/`

Le projet historique est un seul package plat `kpx` (plus `kpx/ui`,
`kpx/term`, `kpx/cli`). Voici les responsabilités identifiées, regroupées
par concern, avec la destination proposée (détaillée en §2).

### Cœur proxy (le moteur)

| Fichier source | Responsabilité | Destination proposée |
|---|---|---|
| `process.go` | Traitement d'une connexion cliente : matching, auth, dial, forward, tunnel, mitm, pipe. **36 ko, à éclater (§3).** | `internal/proxy/processor/*` |
| `process.go` (`findFirstProxy`) + `config.go` (`lastProxies`/`lastMMutex`) | **Sélection d'amont & haute dispo** : sonde les hôtes/proxies *failover*, mémorise le dernier joignable (cache HA). Concern aujourd'hui éclaté entre process.go et la config. | `internal/proxy/upstream/` |
| `request.go` | `ProxyRequest` / `RequestHeader` : lecture/écriture/parsing des messages HTTP. | `internal/proxy/message/` |
| `conn.go` | `TrafficConn`, `ConfigureConn`. **`TimedConn` et `CloseAwareConn` sont abandonnés** (cf. §5). | `internal/proxy/transport/` |
| `chunked.go` | Lecture/écriture HTTP *chunked* (copie de la stdlib instrumentée). | `internal/proxy/transport/` |
| `proxy.go` | `Proxy` : orchestrateur runtime. Mélange **plusieurs concerns** : serveur HTTP + serveur SOCKS (listeners), watch/reload config, génération de tickets Kerberos, ACL, lifecycle. Le **pool de connexions** (`newPooledConn`/`pushConnToPool`/`vacuumPool`/`connPool`) est abandonné (cf. §5). | éclaté : `internal/proxy/server/`, `internal/config/` (watch), `internal/app/` |

### Configuration

| Fichier source | Responsabilité | Destination proposée |
|---|---|---|
| `config.go` | `Conf`/`ConfProxy`/`ConfRule`/`ConfCred`, parsing yaml/json, `check()`, `build()`, `genPac()`, `genCerts()`, **matching `match()`/`resolve()`/`resolvePac()`** et types `ProxyType`. 32 ko. | `internal/config/` (structs + load + validate) et `internal/proxy/router/` (matching runtime) |

### Sécurité / authentification

| Fichier source | Responsabilité | Destination proposée |
|---|---|---|
| `kerberos.go`, `kerberos_store.go` | Store de clients Kerberos par realm/login, génération de tickets. | `internal/service/kerberos/` |
| `kerberos_native_*.go` | Implémentations natives OS (Linux/Windows/other). | `internal/service/kerberos/` (fichiers `*_linux.go` / `*_windows.go` / `*_other.go`) |
| `certs.go`, `certs_manager.go` | CA auto-signée + génération de certificats par host (pour le MITM). | `internal/service/cert/` |
| `password.go` | Clé de chiffrement + `encrypt`/`decrypt` des mots de passe + commande `-e`. | `internal/service/secret/` |

### Plomberie / transverse

| Fichier source | Responsabilité | Destination proposée |
|---|---|---|
| `log.go` | `logInfo`/`logError`/`logTrace`/`logHeader`/`traceInfo`. | **déjà migré** → `internal/service/printer/` (logger.go + request.go) |
| `pac.go` | Exécution PAC via goja. | **déjà migré** → `internal/service/pac/` |
| `mre.go` | `ManualResetEvent`. | `internal/service/event/` (ou supprimé, cf. §5 — remplacé par `context`) |
| `global.go` | Constantes (timeouts, tailles), `Options`, variables globales (`debug`/`trace`/`logger`). | éclaté : constantes près de leur usage, `Options` → `internal/cli/`, globales `debug`/`trace` supprimées |
| `main.go` | `Main()`, parsing des flags (`cmd`), usage/help, `start()`, **self-update** (`update`), `splitHostPort`/`splitUsername`. | éclaté : `internal/cli/` (flags/usage), `internal/update/` (self-update), `internal/app/` (start/wiring), helpers → `internal/config/` ou util |

### UI et terminal (déjà sortis du package plat)

| Source | Responsabilité | Destination proposée |
|---|---|---|
| `kpx/ui/*` | UI console + tview, `TrafficTable`/`TrafficRow`. | `internal/ui/` (le projet a déjà `ui/tui` + `ui/textmode` ; à réconcilier) |
| `kpx/term/*` | Mode raw terminal multi-OS. | déjà couvert par `internal/ui/textmode/` (à comparer) |
| `kpx/cli/main.go` | Point d'entrée binaire. | `cmd/<bin>/` |

---

## 2. Architecture cible

### Choix : hexagonale « légère » (ports & adapters) en couches

Le projet suit déjà cette logique : un cœur (`service/`) sans dépendance UI,
des adaptateurs de présentation (`ui/`), et un orchestrateur (`app/`) qui
câble le tout via le `context` pour le shutdown. On **prolonge** ce style
plutôt que d'imposer une hexagonale dogmatique (pas de sur-abstraction en
interfaces si un seul adapter existe).

Principes :

- **Le domaine ne dépend de rien d'infrastructurel.** `config`/`router`/
  `processor` ne connaissent ni `os.Exit`, ni les flags, ni un logger
  global : ils reçoivent leurs collaborateurs (un `*printer.Printer`, un
  `cert.Manager`, un `kerberos.Store`) par injection — exactement comme
  `pac.NewPac` reçoit déjà son `*printer.Printer`.
- **Les bords sont des ports.** Entrées : listener HTTP, listener SOCKS5.
  Sorties : dialers amont (direct / http-forward / socks), DNS, horloge.
- **Pas de variables globales** (`debug`, `trace`, `logger`, `options`).
  La verbosité descend dans la config ; le logger est le `Printer` injecté.
- **Shutdown par `context`**, pas par `ManualResetEvent` + `os.Exit`
  disséminés (cf. §5).

### Arborescence proposée

```
cmd/<bin>/                 point d'entrée (flags → app.Run(ctx))
internal/
  app/                     orchestrateur : construit les services, lance
                           les listeners, attend le shutdown (existant)
  cli/                     parsing des flags → CmdArgs, usage/help/version
  config/                  3 couches CmdArgs+FileConfig→ProxyConf (cf. §4),
                           load yaml/json, check(), build(), proxy.pac, watch
  proxy/
    router/                matching runtime : (url,host) → rule + []proxy,
                           résolution PAC, cache d'hôtes  (match/resolve)
    message/               ProxyRequest / RequestHeader (lecture/écriture)
    transport/             TrafficConn, chunked, ConfigureConn + un
                           SetReadDeadline ponctuel sur la lecture d'en-têtes
                           (plus de TimedConn ni CloseAwareConn, cf. §5)
    upstream/              sélection d'amont + haute dispo : sonde les hôtes/
                           proxies failover, cache du dernier joignable (HA)
    server/                ports d'entrée : demux HTTP/SOCKS (un seul port
                           possible) + handler web local proxy.pac  (cf. §3.5)
    processor/             traitement par type de proxy  (cf. §3)
  service/
    clock/  printer/  pac/   (existants)
    cert/                  CA + certificats par host (MITM)
    kerberos/              store + clients + natif OS
    secret/                chiffrement des mots de passe
  update/                  self-update (check + download + replace binaire)
  ui/                      console + tview (existant, à réconcilier avec kpx/ui)
```

> Note : `ARCHITECTURE.md` décrit encore la démo « horloge ». Il devra être
> réécrit au fil de la migration (l'ajout/retrait de package est justement
> un des déclencheurs imposés par `CLAUDE.md`). Ce `MIGRATION.md` reste le
> document de pilotage tant que le portage n'est pas terminé.

### Modèle de concurrence (immutabilité / single-thread)

Le proxy est massivement multi-threadé (une goroutine par connexion). La
sûreté repose sur une règle simple, **à faire respecter par une revue
dédiée** de toutes les structs lors du portage :

- **Config immuable après build.** `FileConfig` et `ProxyConf` (et leurs
  sous-structs `Proxy`/`Rule`/`Cred`) ne sont **jamais mutés** une fois
  `build()` terminé. Ils sont partagés en lecture seule par toutes les
  goroutines via un pointeur atomique échangé au reload (le pattern
  `atomic.Pointer[Config]` existant) : aucune verrou nécessaire en lecture,
  et un hot-reload publie un nouvel objet entier au lieu de muter l'ancien.
  Les `*bool` de la cascade (§4.3) sont résolus en `bool` figés au build,
  donc même eux sont en lecture seule au runtime.
- **L'état mutable sort de la config.** Tout ce qui change en cours de route
  ne doit **pas** vivre dans les structs de config (sinon elles ne sont plus
  immuables) : le cache HA (`lastProxies`), le cache de match (`hostsCache`).
  Chacun part dans son composant dédié (`upstream`, `router`) avec une
  synchronisation explicite (mutex) ou un propriétaire unique. Extraire
  `lastProxies`/`hostsCache` de `Config` est précisément ce qui rend
  `ProxyConf` *pleinement* immuable.
- **Un processeur = une connexion, single-thread.** Une instance de
  processeur (l'actuel `Process`) ne sert qu'une connexion et n'est touchée
  que par sa goroutine ; la seule exception est le double pipe (2 goroutines)
  qui opère sur deux `net.Conn` distinctes sans état partagé muté.

Revue à mener **avant de figer les API** des packages `config`/`upstream`/
`router` : classer chaque champ en (a) immuable post-build, (b) mutable
partagé → synchro ou extraction, (c) single-thread.

---

## 3. Refonte de `process.go` : un traitement par type de proxy

`process.go` concentre aujourd'hui toute la logique dans deux méthodes
géantes (`processChannel` pour l'entrée HTTP, `processSocks` pour l'entrée
SOCKS) avec des `switch *firstProxy.Type` et des `if mitm` imbriqués.
L'objectif est de **lister explicitement les cas** et d'avoir **un fichier
par processeur**.

### 3.1 Les types de proxy *de configuration*

Ce sont les valeurs possibles de `ConfProxy.Type` (cf. `config.go`) :

| Type config | Sens | Devient à l'exécution |
|---|---|---|
| `kerberos` | proxy HTTP amont + auth Kerberos (Negotiate) | proc. **http-forward** + authent. kerberos |
| `basic` | proxy HTTP amont + auth Basic (base64) | proc. **http-forward** + authent. basic |
| `anonymous` | proxy HTTP amont sans auth | proc. **http-forward** + authent. anonyme |
| `socks` | proxy SOCKS5 amont (auth optionnelle) | proc. **socks** |
| `direct` | connexion directe à l'origine | proc. **direct** |
| `none` | rejet systématique (400) | proc. **none** |
| `pac` | **méta-type** : un script PAC décide à l'exécution | résolu en `direct`/`anonymous`/`socks` par le router (§2), **n'a pas de processeur propre** |

Point clé demandé : **anonymous, basic et kerberos partagent le même
transport amont** (un proxy HTTP : on dial le proxy, on envoie la requête en
URL absolue avec éventuellement `Proxy-Authorization`, ou un `CONNECT`).
Ils **ne diffèrent que par l'authentificateur**. On factorise donc le
transport (`http-forward`) et on paramètre l'auth — sans pour autant
mélanger anonymous et basic dans un même chemin de code (ce sont des
stratégies d'auth distinctes).

### 3.2 Les axes orthogonaux

Le « type de proxy » ne suffit pas à déterminer le traitement : trois axes
se combinent.

- **Axe A — connecteur amont (comment joindre le hop suivant) :**
  `none` · `direct` · `http-forward` · `socks`.
- **Axe B — authentificateur (uniquement pour `http-forward` et `socks`) :**
  `anonyme` · `basic` (par-conf / par-user) · `kerberos` (par-conf mot de
  passe / par-conf natif OS / par-user). C'est `computeAuthPerConf` /
  `computeAuthPerUser` aujourd'hui.
- **Axe C — mode de transport (que faire de la connexion établie) :**
  - `http` — forward requête/réponse, keep-alive, chunked ;
  - `tunnel` — `CONNECT` puis double pipe aveugle (duplex) ;
  - `mitm` — `CONNECT` puis terminaison TLS des deux côtés + boucle
    requête/réponse déchiffrée (uniquement si `rule.Mitm`) ;
  - `direct-connect` — variante d'upgrade (le chemin `/~/https://…`,
    `directToConnect`).

C'est l'axe C qui matérialise la demande « si mitm est activé, ce n'est pas
le même processeur » : `tunnel` et `mitm` sont deux processeurs séparés,
même connecteur amont.

### 3.3 Liste explicite des processeurs (produit cartésien utile)

En croisant les axes, voici **les processeurs concrets** à écrire. On ne
garde que les combinaisons réellement atteignables (p. ex. `none` ignore B
et C ; `pac` n'apparaît pas).

**Entrée serveur HTTP :**

1. `none` — répond `400 Bad Request` et ferme. *(none.go)*
2. `direct` + `http` — dial origine, forward HTTP (keep-alive/SSL opt.). *(direct_http.go)*
3. `direct` + `tunnel` — `CONNECT`, double pipe aveugle. *(direct_tunnel.go)*
4. `direct` + `mitm` — `CONNECT`, TLS terminé des deux côtés, boucle. *(direct_mitm.go)*
5. `http-forward` + `http` — dial proxy amont, forward URL absolue + `Proxy-Authorization`, keep-alive. *(forward_http.go)*
6. `http-forward` + `tunnel` — `CONNECT` au proxy amont puis pipe aveugle. *(forward_tunnel.go)*
7. `http-forward` + `mitm` — `CONNECT` au proxy amont, TLS des deux côtés, boucle. *(forward_mitm.go)*
8. `http-forward` + `direct-connect` — chemin d'upgrade `/~/` puis forward. *(forward_directconnect.go)*
9. `socks` + `http`/`tunnel` — dial via SOCKS5 amont puis pipe. *(socks_http.go / partagé)*

**Entrée serveur SOCKS5 :** (toujours en tunnel/pipe, pas de notion HTTP)

10. `socks-server` + `direct` — dial origine, double pipe. *(socksin_direct.go)*
11. `socks-server` + `socks` — dial via SOCKS5 amont, double pipe. *(socksin_socks.go)*

> L'authentificateur (axe B) **n'ajoute pas de fichier** : c'est une
> stratégie injectée dans les processeurs 5–9 et 11. On évite ainsi
> l'explosion combinatoire (3 auth × 4 modes) en gardant l'auth orthogonale.

### 3.4 Décomposition recommandée (pour éviter la duplication)

Plutôt que 11 copies du squelette, on recommande :

```
internal/proxy/processor/
  processor.go        interface Processor { Handle(ctx, client) error } + dispatch
                      (choisit le processeur depuis rule+proxy+header)
  none.go             #1
  direct.go           #2-#4 (branche sur le mode A=direct)
  forward.go          #5-#8 (branche sur le mode, A=http-forward)
  socks.go            #9 (+ entrée SOCKS #10-#11)
  authenticator.go    stratégies d'auth (axe B) : anonymous/basic/kerberos,
                      par-conf & par-user, natif — issu de computeAuth*
  transport_http.go   brique « forward requête/réponse » réutilisée par 2 & 5
  transport_tunnel.go brique « double pipe » réutilisée par 3,6,9,10,11
  transport_mitm.go   brique « TLS + boucle » réutilisée par 4 & 7
```

Chaque processeur reste court : il établit la connexion amont (axe A, avec
l'auth de l'axe B) puis délègue à une brique de transport (axe C). C'est le
remplacement direct des deux grosses méthodes actuelles. **À valider avant
de coder** (phase 1 de `CLAUDE.md`) : on peut aussi partir sur « un fichier
strictement par type » (none/direct/forward/socks) avec le mode en
paramètre interne — plus proche de la demande littérale « un fichier par
type de proxy », au prix de fonctions un peu plus longues.

### 3.5 Classification à l'entrée : web PAC local et demux HTTP/SOCKS

Deux concerns qui ne sont **pas** des processeurs de proxy mais relèvent de
la même brique — la **classification de la connexion au niveau du port**.
Ils vivent dans `internal/proxy/server`, en façade du dispatch, et n'ont ni
`switch` de type de proxy ni `simulateConnect` (cf. la frontière
connecteur/transport du §3.4 : le port classe, puis route).

**a) Serveur web local (`proxy.pac`).** Aujourd'hui entrelacé dans
`processChannel` (process.go:99) : une requête est classée sur la *forme*
de son URL. URL absolue (`http://host/…`) ou `CONNECT host:port` → chemin
proxy ; URL en *origin-form* (commence par `/`) → serveur local, qui ne
sert que `GET /proxy.pac` (`webServer`, process.go:549). Dans l'archi cible
c'est un **plan de contrôle local**, frère du dispatch proxy, pas un
connecteur amont :

```
entrée HTTP → lire ligne+headers → classer :
   ├─ origin-form ("/...")     → handler web local (proxy.pac, futur /status…)
   └─ absolute-form / CONNECT  → dispatch proxy (router → connecteur → transport)
```

Le handler web détient un accès à la **config injectée** (pas une copie
figée : la config hot-reload, il lit le PAC courant à chaque requête) et
**répond localement**, sans notion d'amont.

**b) Demux HTTP/SOCKS sur un seul port (optionnel).** Techniquement
faisable et propre (pattern `cmux`) : on *peek* le 1ᵉʳ octet de la
connexion acceptée. Les plages ne se chevauchent jamais :

- SOCKS5 = `0x05`, SOCKS4 = `0x04` (octet de version) ;
- HTTP = méthode en ASCII imprimable (`G`,`P`,`C`,`H`,… `0x41`–`0x5A`).

```
listener unique → Accept → peek(1 octet) :
   ├─ 0x04 / 0x05   → entrée SOCKS
   └─ lettre ASCII  → entrée HTTP  (puis classification (a) : local vs proxy)
```

Mécaniquement, deux points, tous deux disponibles :

1. **Rendre l'octet peeké** — envelopper la `net.Conn` dans un `prefixConn`
   (un `bufio.Reader` re-préfixant les octets lus, ou le `io.MultiReader`
   déjà utilisé dans `forwardStream`) ; le protocole en aval voit le flux
   complet, octet de version inclus.
2. **Piloter la lib SOCKS sans `ListenAndServe`** — `txthinking/socks5`
   expose `Negotiate(rw)` (server.go:87) et `GetRequest(rw)` (server.go:132)
   en **public**. On fait donc `accept → peek → (si SOCKS)
   server.Negotiate(prefixConn); req := server.GetRequest(...); dispatch`,
   sans la boucle `ListenAndServe` (qui monopolise un port). L'actuel
   `TCPHandle` devient un simple appelé du dispatch.

Cela unifie **proxy HTTP + serveur PAC + SOCKS sur un seul port** sous une
seule abstraction `server` : un *demux* en façade, les entrées derrière.
La config pourrait passer de `port` + `socksPort` à un `port` unique, en
gardant éventuellement les deux pour rétro-compat / isolation de SOCKS.

Réserves : si un jour le proxy est exposé en TLS entrant, le handshake
commence par `0x16` (distinguable aussi, mais kpx ne fait pas de TLS
inbound aujourd'hui) ; le coût du peek d'1 octet est négligeable ; le seul
vrai coût est de ne plus utiliser `ListenAndServe` et de gérer soi-même la
boucle `Accept` (qu'on réécrit de toute façon : ACL, `context`, shutdown).

---

## 4. Modèle de configuration : trois couches + cascade tri-state

### 4.1 Le problème

Aujourd'hui une seule famille de structs (`Conf`/`ConfProxy`/`ConfRule`/
`ConfCred` dans `config.go`) **mélange trois natures de données** :

- ce qui est **lu du YAML/JSON** (champs exportés : `Type`, `Host`, `Port`,
  `Verbose`, `Mitm`…) ;
- ce qui est **calculé/dérivé** au build (champs non exportés : `name`,
  `cred`, `typeValue`, `regex`, `pacRegex`, `pacJs`, `pacProxy`, `isUsed`,
  `pacRuntime`, `pacProxies`…) ;
- et, via le global `options` (`Options`), ce qui vient de la **ligne de
  commande** et qui est fusionné de force dans `Conf` (cf. `NewConfig`).

On en profite pour séparer ces couches en types distincts.

### 4.2 Les trois couches

Pipeline explicite **parse → build** matérialisé par trois types, au lieu
d'un struct fourre-tout :

| Couche | Type proposé | Contenu | Provenance |
|---|---|---|---|
| Arguments CLI | `CmdArgs` | flags ligne de commande | remplace `Options` (`global.go`) |
| Fichier | `FileConfig` (+ `FileProxy`/`FileRule`/`FileCred`) | **uniquement** ce qui est lu du fichier, tags yaml/json **identiques à aujourd'hui** | parse pur de `Conf` & co |
| Résolu | `ProxyConf` (+ `Proxy`/`Rule`/`Cred`) | modèle runtime construit à partir de `CmdArgs` + `FileConfig`, champs effectifs **déjà résolus** (plus de `*bool` ni de nil) + champs calculés | `build()` |

> **Nom de la couche fichier :** je recommande **`FileConfig`** plutôt que
> `YamlConfig` — le format peut être YAML **ou** JSON (`readFromFile`
> détecte les deux), donc « Yaml » serait un abus. `ConfigFile` conviendrait
> aussi ; à trancher.

Mapping indicatif des champs actuels (à valider champ par champ en phase 1) :

- `ConfProxy` → `FileProxy` (`Type`,`Host`,`Port`,`Ssl`,`Spn`,`Realm`,
  `Credential`,`Credentials`,`Pac`,`PacOrder`,`Url`,`Verbose`,`Debug`,
  `Trace`) **+** `Proxy` (résolu : `name`,`typeValue`,`cred`,`pacRegex`,
  `pacJs`,`pacProxy`,`isUsed`,`pacRuntime`, et les booléens effectifs).
- `ConfRule` → `FileRule` (`Host`,`Proxy`,`Dns`,`Verbose`,`Debug`,`Trace`,
  `Mitm`) **+** `Rule` (résolu : `regex`, booléens effectifs).
- `ConfCred` → `FileCred` (`Login`,`Password`) **+** `Cred` (résolu :
  `name`,`isNull`,`isPerUser`,`isUsed`,`isNative`).
- les champs calculés de `Conf` (`pacProxy`,`pacProxies`,
  `experimental*`,…) partent dans `ProxyConf`.

Le runtime (router, processeurs) ne manipule plus que `ProxyConf` : il lit
des valeurs déjà résolues, il ne refait pas la cascade ni le parsing.

### 4.3 Cascade tri-state : `debug` / `verbose` / `trace` / `mitm`

Objectif : pouvoir régler ces drapeaux à **chaque niveau**. Ordre de
priorité **décidé** (le plus prioritaire à gauche) :

```
args CLI  >  global (fichier)  >  proxy (cible)  >  proxy pac  >  rule
(le plus prioritaire)                                      (le moins prioritaire)
```

C'est volontairement **l'inverse du « plus spécifique gagne »** : ce sont
des bascules opérateur/débogage, donc un réglage `args` ou `global`
l'emporte sur un réglage de `rule`. Conséquence assumée : une `rule` ne peut
**pas** désactiver ce qu'un niveau supérieur a activé ; elle ne sert qu'à
activer là où les niveaux supérieurs n'ont rien fixé. (`args` ne porte que
`debug`/`trace`/`verbose` — il n'y a pas de flag CLI pour `mitm`, qui se
résout donc via global > proxy > pac > rule.)

- **Représentation tri-state = `*bool`** (`nil` = hérite, `true`/`false` =
  explicite). C'est déjà ce que fait `ConfProxy.Verbose`/`ConfRule.Verbose`
  aujourd'hui ; on **généralise** à `debug`, `trace` et `mitm`, et on
  l'ajoute au niveau global (où `Debug`/`Trace`/`Verbose` sont aujourd'hui
  de simples `bool`, et `Mitm` un `bool` au niveau rule).
- **Résolution dans `build()`** : pour chaque rule/proxy, la valeur
  effective est la **première non-`nil`** en parcourant les niveaux du plus
  au moins prioritaire (`args` d'abord) ; à défaut, `false`. Le résultat est
  un `bool` figé stocké dans `ProxyConf`. Une petite fonction
  `coalesce(args, global, proxy, pac, rule *bool) bool` (renvoie le premier
  non-`nil`) factorise les 4 drapeaux.
- **Implication conservée** : `trace ⇒ debug ⇒ verbose` (déjà appliqué
  dans `NewConfig`) est réappliquée après la cascade, au même endroit.
- Le **niveau pac** correspond au proxy de type `pac` qui a résolu la
  cible : aujourd'hui `resolve()` recopie déjà `Verbose: rule.Verbose` dans
  le proxy temporaire ; on porte ces réglages comme un niveau sous le proxy
  cible.

### 4.4 Compatibilité ascendante (impératif)

La refonte **ne doit rien casser** pour les fichiers et args existants :

- **Tags yaml/json inchangés** sur `FileConfig`/`FileProxy`/… : un fichier
  actuel parse à l'identique (c'est le seul contrat observable côté
  fichier). Le découpage en types est interne.
- **`bool` → `*bool`** au niveau global (`debug`/`trace`/`verbose`) et pour
  `mitm` : sans impact de parsing — `debug: true` se désérialise dans un
  `*bool`, une clé absente donne `nil`. Les fichiers actuels restent
  valides.
- **`CmdArgs` garde les mêmes flags** (`-d/-t/-v`, `--ui`, forme proxy
  unique en argument positionnel, etc.).
- **Précédence (décidée) préserve le comportement actuel :** `args` est le
  niveau **le plus prioritaire** (§4.3), donc un flag CLI `-d/-t/-v` prime
  toujours — équivalent au `conf.Debug = conf.Debug || options.Debug || …`
  d'aujourd'hui où le CLI force l'activation. Et comme aucun fichier existant
  ne règle `debug`/`trace` à un niveau plus fin que `global`, la cascade
  donne exactement les mêmes valeurs effectives qu'avant sur les configs
  actuelles ; la granularité fine (proxy/pac/rule) n'est qu'un ajout pour
  les configs *nouvelles*.

---

## 5. Méthodes candidates à l'abandon

Repérées pendant l'inventaire, à ne **pas** reporter telles quelles (l'archi
context-cancellation les remplace) — à confirmer item par item :

- `Proxy.init()` / `mre.go` (`ManualResetEvent`) : remplacés par `context`
  + goroutines lancées depuis `app`. Le watch/reload peut utiliser un
  simple `chan` + `context`.
- `Proxy.exit()` / `os.Exit` disséminés : le shutdown remonte une erreur /
  annule le `context`, `main` décide du code de sortie.
- Variables globales `debug`/`trace`/`logger`/`options` : remplacées par
  des champs de config injectés et le `Printer`.
- `logInit`/`logDestroy`/`logWriter`/`logFlush` : déjà couverts par
  `printer.New`/`Run`/`Flush` (cf. README printer).

### Décisions actées : connexions & timeouts

Tranché (plus « candidat », ce sont des suppressions décidées) :

- **`TimedConn` supprimé**, ainsi que les knobs `idleTimeout` **et**
  `closeTimeout`. Motifs :
  - `idleTimeout` est conceptuellement faux : un `idleTimeout > 0` fait
    qu'une simple requête HTTP attend la fin du délai d'inactivité avant de
    fermer la connexion — inacceptable. On retire la feature.
  - `closeTimeout` est vestigial : son seul usage vivant
    ([process.go:743-744](process.go), après le `io.Copy`) est toujours
    écrasé par le `setTimeout` suivant ou suivi d'un `close` avant le
    prochain `Read` — il ne gouverne donc jamais un `Read` réel.
  - Reste à conserver **uniquement `connectTimeout`** : il borne le dial
    (`dialer.Timeout`) et la **lecture des en-têtes**. Ce dernier besoin se
    couvre par un simple `conn.SetReadDeadline(now + connectTimeout)` autour
    de `readRequestHeaders`, **sans wrapper** → `TimedConn` disparaît.
- **`CloseAwareConn` supprimé** : ne sert qu'au pool (son `Reset`/`ReOpen`
  ne se déclenche que sur réutilisation depuis le pool) ; sans pool, c'est
  un wrapper inerte.
- **Pool de connexions amont supprimé** (`connPool`, `newPooledConn`,
  `pushConnToPool`, `vacuumPool`, `PooledConnection*`, le knob
  `experimental: connection-pools`) : erreur conceptuelle. La gestion du
  *pool / keep-alive* est laissée au **client qui appelle le proxy**, pas à
  kpx. Conséquence positive : le connecteur `http-forward` se simplifie
  (plus de branche « reuse depuis le pool », plus de plomberie
  `authorizationContext` comme clé de pool, plus d'optimisation
  « connexion réutilisée déjà authentifiée »). Le package
  `internal/proxy/pool` **n'est donc pas créé**.

---

## 6. Stratégie de migration (ordre des étapes)

Migration **feuille d'abord** : on porte ce qui n'a pas de dépendance avant
ce qui en a, pour que chaque étape compile (`go build`/`go vet`) sans stub.
Chaque étape = un cycle « proposer le découpage → valider → copier ».

**Pendant la migration de chaque service/composant, faire une analyse :**
si des adaptations, corrections ou optimisations apparaissent, les **lister
dans `IDEAS.md`** (à la racine) plutôt que de les appliquer au vol. La phase
de copie reste fidèle (cf. `CLAUDE.md` : « copier avec transformation
minimale, résister à l'envie de redesigner ») ; `IDEAS.md` est l'exutoire où
ces améliorations sont capturées pour être traitées **plus tard**, sans
parasiter le portage en cours. Une entrée = composant concerné + idée +
pourquoi (et, si utile, un lien vers le code).

1. **`internal/service/secret`** (`password.go`) — feuille pure (clé +
   encrypt/decrypt). Débloque le décodage des mots de passe en config.
2. **`internal/config` — modèle 3 couches & load (§4)** : `FileConfig`/
   `FileProxy`/`FileRule`/`FileCred` (parse yaml/json à tags **identiques**),
   `ProxyType`, `check()`, puis `build()` qui produit `ProxyConf` à partir de
   `CmdArgs`+`FileConfig` (cascade tri-state `debug`/`verbose`/`trace`/`mitm`).
   Pas de runtime, pas de réseau. Utilise `secret` pour `decrypt`. `CmdArgs`
   peut être porté ici ou en amont avec l'étape 13 (`cli`).
3. **`internal/service/cert`** (`certs.go`, `certs_manager.go`) — feuille
   crypto, requise par le MITM et par `config.genCerts()`.
4. **`internal/service/kerberos`** (`kerberos*.go`, store + natif OS) —
   feuille (dépend de la lib gokrb5), requise par l'auth.
5. **`internal/proxy/transport`** (`conn.go` + `chunked.go`) — `TrafficConn`,
   `ConfigureConn`, *chunked*, et un helper `SetReadDeadline(connectTimeout)`
   pour borner la lecture d'en-têtes. **Pas** de `TimedConn` ni
   `CloseAwareConn` (cf. §5). Dépend de la UI (`TrafficRow`) → à réconcilier
   avec `internal/ui` au passage.
6. **`internal/proxy/message`** (`request.go`) — `ProxyRequest`. Dépend de
   `transport` et du `Printer`.
7. **`internal/proxy/router`** (extrait de `config.go` : `match`/`resolve`/
   `resolvePac`/cache d'hôtes + `genPac`). Dépend de `config` et `pac`.
8. **`internal/proxy/upstream`** — **sélection d'amont & HA** : extraction de
   `findFirstProxy` (process.go) + de l'état `lastProxies`/`lastMMutex`
   (sorti de `Config`, cf. modèle de concurrence). Sonde les hôtes/proxies
   failover et cache le dernier joignable. Dépend de `transport` (dial) et
   `config`. Les connecteurs (axe A, §3) délèguent ici le choix du host:port.
9. **`internal/proxy/processor`** — **le gros morceau (§3)**. Éclatement de
   `process.go`. Dépend de 4,5,6,7,8 + `cert`. **Pas de pool** : le
   connecteur `http-forward` dial à chaque fois (cf. §5). À faire en
   sous-étapes : d'abord l'interface + dispatch + `none`/`direct`, puis
   `forward` + authentificateurs, puis `tunnel`/`mitm`, puis l'entrée SOCKS.
10. **`internal/proxy/server`** (extrait de `proxy.go` : listeners HTTP +
    SOCKS5, ACL, boucle d'`Accept`). Câble le processor.
11. **`internal/config` — watch/reload** (extrait de `proxy.go` :
    `watch1`/`watch2`/`reload`, via `context` au lieu de `ManualResetEvent`).
12. **`internal/update`** (extrait de `main.go` : `update()` + helpers JSON).
13. **`internal/cli`** (`main.go` : flags → `CmdArgs`, usage/help/version,
    `splitHostPort`/`splitUsername`) + câblage final dans **`internal/app`**
    et le `cmd/<bin>/` d'entrée.
14. **Réconciliation UI** (`kpx/ui` ↔ `internal/ui/{tui,textmode}`) et
    réécriture de `ARCHITECTURE.md` pour refléter le moteur proxy au lieu de
    la démo horloge.

À chaque étape : mettre à jour le `README.md` du package de destination (et
sa section `## Limitations`), `ARCHITECTURE.md` si l'étape ajoute/retire un
package — comme imposé par `CLAUDE.md` — et consigner dans `IDEAS.md` toute
adaptation/correction/optimisation repérée pendant l'analyse (cf. ci-dessus).

**Documentation au fil de l'eau.** Améliorer la doc à mesure que le code se
met en place, pour rendre l'intention plus claire — mais **rester concis** :
on explique le *pourquoi*, les cas d'usage et les cas particuliers (ce que
le code ne dit pas de lui-même), on ne paraphrase pas le *comment*. Une
explication courte et juste vaut mieux qu'un long pavé ; mieux vaut nommer
explicitement un cas limite qu'enrober le tout de généralités.

---

## 7. Tests (après la migration du code)

**À planifier une fois le code stabilisé, pas pendant le portage.** Une fois
les packages en place, on voudra une couverture systématique de **tous les
cas identifiés dans ce document** :

- **Tests unitaires** — par cas, sur les composants isolés :
  - la matrice de processeurs du §3.3 (none / direct / forward / socks ×
    http / tunnel / mitm / direct-connect) ;
  - la cascade tri-state du §4.3 (`debug`/`verbose`/`trace`/`mitm`, priorité
    `args > global > proxy > pac > rule`, + implication
    `trace ⇒ debug ⇒ verbose`, + `args` prioritaire = compat CLI) ;
  - le matching/router et la résolution PAC ;
  - la sélection d'amont & HA (`upstream`) : failover hôte/proxy, cache du
    dernier joignable ;
  - la classification d'entrée du §3.5 (demux HTTP/SOCKS sur 1 octet,
    handler web `proxy.pac`) ;
  - le parsing `FileConfig` à partir d'exemples yaml **et** json existants
    (garde-fou de compatibilité ascendante, §4.4).
- **Tests d'intégration** — les mêmes cas bout-en-bout, avec une infra réelle
  (proxies amont HTTP/SOCKS, Kerberos, MITM) montée via docker-compose, comme
  anticipé pour les composants `service/` dans `ARCHITECTURE.md`.

Cette section listera les cas précis à couvrir au moment venu ; elle est
posée ici pour que rien de ce qui a été identifié pendant la migration ne
soit oublié côté tests.
