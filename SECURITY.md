# Signaler une faille de sécurité

VLPM gère des données nominatives d'agents de police municipale : historique de
conduite, incidents, photos de constat. Une faille peut donc exposer bien plus
qu'un simple parc automobile.

## Comment signaler

**N'ouvrez pas d'issue publique.** Écrivez à **assopmfrance@gmail.com** en
indiquant :

- ce que la faille permet de faire ;
- les étapes pour la reproduire ;
- la version concernée (`vlpm aide` ou `GET /api/v1/version`).

Nous accusons réception sous quelques jours. L'association est bénévole : nous
ne promettons pas de délai de correction, mais un problème permettant d'accéder
aux données d'un service sans authentification sera traité en priorité.

Merci de nous laisser un délai raisonnable avant toute publication.

## Ce qui relève de l'installation, pas du logiciel

Les points suivants ne sont pas des failles applicatives, mais des erreurs de
déploiement que le README documente :

- **instance en HTTP** : les mots de passe circulent en clair, et ni le scan
  QR ni le mode hors ligne ne fonctionnent. Placez un reverse proxy HTTPS ;
- **dossier de données lisible par tous** : les installateurs le posent en
  `0750` sous un compte système dédié ;
- **sauvegardes conservées sur le seul disque du serveur** : elles ne
  protègent pas d'une panne matérielle ni d'un vol de la machine ;
- **horloge serveur non synchronisée** : le second facteur cesse de
  fonctionner si elle dérive de plus d'une minute.

## Portée

Sont concernés le code de ce dépôt et les binaires publiés dans les
*releases*. Les instances installées par les communes relèvent de leur propre
responsable de traitement.
