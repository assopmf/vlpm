// VLPM — application web.
//
// Page unique, sans framework ni étape de compilation : le fichier servi est
// le fichier écrit. Une association qui reprend le projet doit pouvoir corriger
// un libellé avec un éditeur de texte, sans chaîne d'outils Node.

import { api, session, brancherExpiration, ErreurAPI } from './api.js';
import {
  empiler, abandonner, fileEnAttente, nombreEnAttente, synchroniser,
  memoriserParc, parcMemorise, surChangement, demarrerSynchronisation, nouvelleCle,
} from './horsligne.js';
import {
  esc, icones, nombre, dateHeure, dateCourte, euros, badge, message, vide,
  carteVehicule, LIBELLES_STATUT, LIBELLES_INCIDENT, LIBELLES_GRAVITE,
  LIBELLES_ENTRETIEN,
} from './ui.js';

const app = document.getElementById('app');

// --- Routeur ---------------------------------------------------------------

const routes = [
  [/^\/connexion$/, () => vueConnexion()],
  [/^\/$/, () => vueAccueil()],
  [/^\/scan$/, () => vueScan()],
  [/^\/scan\/(.+)$/, (m) => vueScan(m[1])],
  [/^\/vehicule\/(\d+)$/, (m) => vueFicheVehicule(Number(m[1]))],
  [/^\/vehicule\/(\d+)\/modifier$/, (m) => vueFormVehicule(Number(m[1]))],
  [/^\/parc\/nouveau$/, () => vueFormVehicule(null)],
  [/^\/prendre\/(\d+)$/, (m) => vuePriseEnCompte(Number(m[1]))],
  [/^\/restituer\/(\d+)$/, (m) => vueRestitution(Number(m[1]))],
  [/^\/historique$/, () => vueHistorique()],
  [/^\/incidents$/, () => vueIncidents()],
  [/^\/agents$/, () => vueAgents()],
  [/^\/journal$/, () => vueJournal()],
  [/^\/conservation$/, () => vueConservation()],
  [/^\/reglages$/, () => vueReglages()],
  [/^\/attente$/, () => vueFileAttente()],
];

// aller navigue sans rechargement.
// Renvoie la promesse du rendu : un appelant qui veut afficher un message sur
// l'écran d'arrivée doit l'attendre, sinon il écrit dans le DOM sortant.
export function aller(chemin, remplacer = false) {
  if (remplacer) history.replaceState({}, '', chemin);
  else history.pushState({}, '', chemin);
  return rendre();
}

async function rendre() {
  const chemin = location.pathname;

  if (!session.connecte() && chemin !== '/connexion') {
    // On mémorise la destination : après connexion, un lien de QR code doit
    // aboutir sur le bon véhicule et pas sur l'accueil.
    sessionStorage.setItem('vlpm.destination', chemin + location.search);
    return aller('/connexion', true);
  }
  if (session.connecte() && chemin === '/connexion') return aller('/', true);

  for (const [motif, vue] of routes) {
    const m = chemin.match(motif);
    if (m) {
      try {
        await vue(m);
      } catch (err) {
        afficherErreurFatale(err);
      }
      window.scrollTo(0, 0);
      return;
    }
  }
  vue404();
}

window.addEventListener('popstate', rendre);

// Délégation : tout lien interne est intercepté par le routeur.
document.addEventListener('click', (e) => {
  const lien = e.target.closest('a[href^="/"]');
  if (lien && !lien.hasAttribute('download') && !lien.target) {
    e.preventDefault();
    aller(lien.getAttribute('href'));
  }
});

brancherExpiration(() => aller('/connexion', true));

// --- Gabarit ---------------------------------------------------------------

function poser(html, { nav = true } = {}) {
  app.innerHTML = (nav ? barreNavigation() : '')
    + `<div class="page">${bandeauReseau()}${html}</div>`;
}

// bandeauReseau signale l'absence de connexion et les opérations en attente.
// L'agent doit savoir en permanence si ce qu'il saisit part vraiment.
function bandeauReseau() {
  const file = fileEnAttente();
  if (navigator.onLine && !file.length) return '';

  if (!navigator.onLine) {
    return `<div class="bandeau hors-ligne">${icones.horsLigne}
      <div><strong>Hors ligne</strong> — vos saisies sont enregistrées
      ${file.length ? `(${file.length} en attente)` : ''} et transmises au retour du réseau.</div></div>`;
  }

  // En ligne avec des saisies restantes : soit elles attendent leur tour, soit
  // le serveur les a refusées. Annoncer « transmission en cours » dans le
  // second cas laisserait croire que la situation va se régler seule.
  const refusees = file.filter((e) => e.erreur).length;
  if (refusees) {
    return `<a class="bandeau refus" href="/attente">${icones.alerte}
      <div><strong>${refusees} saisie${refusees > 1 ? 's' : ''} refusée${refusees > 1 ? 's' : ''}</strong>
      — votre intervention est nécessaire.</div></a>`;
  }
  return `<a class="bandeau attente" href="/attente">${icones.attente}
    <div><strong>${file.length} opération${file.length > 1 ? 's' : ''} en attente</strong>
    — transmission en cours.</div></a>`;
}

function barreNavigation() {
  const chemin = location.pathname;
  const actif = (p) => (chemin === p ? ' aria-current="page"' : '');
  const liens = [
    ['/', icones.accueil, 'Accueil'],
    ['/scan', icones.qr, 'Scanner'],
    ['/historique', icones.historique, 'Historique'],
  ];
  if (session.peut('chef')) liens.push(['/incidents', icones.alerte, 'Incidents']);
  liens.push(['/reglages', icones.reglages, 'Réglages']);

  return `<nav class="nav">${liens
    .map(([href, icone, libelle]) =>
      `<a href="${href}"${actif(href)}>${icone}<span>${libelle}</span></a>`)
    .join('')}</nav>`;
}

function retour(href, libelle = 'Retour') {
  return `<a class="retour" href="${href}">${icones.fleche} ${esc(libelle)}</a>`;
}

function chargement() {
  poser(`<div class="vide"><p>Chargement…</p></div>`);
}

function afficherErreurFatale(err) {
  const texte = err instanceof ErreurAPI
    ? err.message
    : "Une erreur inattendue est survenue.";
  poser(message('erreur', texte) +
    `<button class="btn secondaire" data-action="recharger">Réessayer</button>`);
  app.querySelector('[data-action="recharger"]')?.addEventListener('click', rendre);
}

function vue404() {
  poser(`<h1>Page introuvable</h1>
    <p class="discret" style="margin:12px 0 20px">Cette adresse ne correspond à aucun écran.</p>
    <a class="btn" href="/">Revenir à l'accueil</a>`);
}

// surClic branche la délégation d'événements sur le conteneur courant.
function surClic(gestionnaire) {
  app.addEventListener('click', (e) => {
    const cible = e.target.closest('[data-action]');
    if (!cible || !app.contains(cible)) return;
    gestionnaire(cible.dataset.action, cible.dataset, e, cible);
  });
}

// afficherMessage insère un message en tête de la zone prévue.
function poserMessage(type, texte) {
  const zone = app.querySelector('#zone-message');
  if (zone) zone.innerHTML = message(type, texte);
  zone?.scrollIntoView({ block: 'nearest' });
}

// enAttente désactive un bouton pendant l'appel réseau, pour éviter le double
// envoi d'une prise en compte sur un réseau mobile lent.
async function enAttente(bouton, tache) {
  const texte = bouton.innerHTML;
  bouton.disabled = true;
  bouton.innerHTML = 'Veuillez patienter…';
  try {
    return await tache();
  } finally {
    bouton.disabled = false;
    bouton.innerHTML = texte;
  }
}

// --- Connexion -------------------------------------------------------------

function vueConnexion() {
  poser(`
    <div class="connexion">
      <div class="logo">${icones.bouclier}</div>
      <h1>VLPM</h1>
      <p class="sous-titre">Gestion du parc automobile</p>
      <div id="zone-message"></div>
      <form id="form-connexion" autocomplete="on">
        <div class="champ">
          <label for="matricule">Matricule</label>
          <input type="text" id="matricule" name="username" autocomplete="username"
                 autocapitalize="none" spellcheck="false" required>
        </div>
        <div class="champ">
          <label for="motdepasse">Mot de passe</label>
          <input type="password" id="motdepasse" name="password"
                 autocomplete="current-password" required>
        </div>
        <button class="btn" type="submit">Se connecter</button>
      </form>
    </div>`, { nav: false });

  const form = app.querySelector('#form-connexion');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const bouton = form.querySelector('button');
    await enAttente(bouton, async () => {
      try {
        const rep = await api.connexion(
          form.matricule.value.trim(), form.motdepasse.value);
        session.ouvrir(rep.token, rep.user);
        const destination = sessionStorage.getItem('vlpm.destination');
        sessionStorage.removeItem('vlpm.destination');
        if (rep.user.must_change_password) {
          await aller('/reglages');
          poserMessage('attention',
            'Votre mot de passe est provisoire. Changez-le dès maintenant.');
          return;
        }
        await aller(destination || '/');
      } catch (err) {
        poserMessage('erreur', err.message);
        form.motdepasse.value = '';
        form.motdepasse.focus();
      }
    });
  });
  form.matricule.focus();
}

// --- Accueil ---------------------------------------------------------------

let filtreParc = 'tous';

async function vueAccueil() {
  chargement();
  const u = session.utilisateur();

  let stats;
  let moi = {};
  let vehicules;
  let instantane = null;
  try {
    [stats, moi] = await Promise.all([api.stats(), api.moi()]);
    vehicules = await api.vehicules('tous');
    memoriserParc(vehicules, stats);
  } catch (err) {
    // Hors ligne : on affiche le dernier état connu plutôt qu'un écran vide.
    // Toute autre erreur remonte normalement.
    if (!(err instanceof ErreurAPI) || err.statut !== 0) throw err;
    instantane = parcMemorise();
    if (!instantane) {
      poser(message('erreur',
        "Vous êtes hors ligne et aucune donnée n'a encore été enregistrée sur cet appareil. "
        + "Connectez-vous une fois au réseau pour pouvoir travailler ensuite sans."));
      return;
    }
    stats = instantane.stats;
    vehicules = instantane.vehicules;
  }
  if (filtreParc !== 'tous') {
    vehicules = vehicules.filter((v) => v.statut === filtreParc);
  }

  const vieilleteInstantane = instantane
    ? message('info', `Données du ${dateHeure(instantane.date)}, dernière synchronisation réussie.`)
    : '';

  // Hors ligne, /moi est injoignable : le véhicule détenu se retrouve dans
  // l'instantané, à partir de la sortie ouverte au nom de l'agent.
  const enMain = moi.checkout_en_cours
    || vehicules.find((v) => v.checkout_en_cours?.user_id === u.id)?.checkout_en_cours;
  const banniere = enMain ? `
    <div class="carte" style="border-color:var(--marine)">
      <div class="entre-deux" style="margin-bottom:12px">
        <div class="pile">
          <strong>Vous détenez le véhicule ${esc(enMain.vehicle_code)}</strong>
          <span class="discret">Depuis ${dateHeure(enMain.started_at)} — ${nombre(enMain.km_start)} km au départ</span>
        </div>
      </div>
      <button class="btn" data-action="restituer" data-checkout="${enMain.id}">
        ${icones.sortie} Restituer le véhicule</button>
    </div>` : '';

  const tuile = (classe, icone, libelle, valeur, statut) => `
    <button class="tuile ${classe}" data-action="filtrer" data-statut="${statut}">
      <div class="haut"><div class="pastille">${icone}</div>
        <div class="libelle">${libelle}</div></div>
      <div class="valeur">${valeur}</div>
    </button>`;

  const filtres = [
    ['tous', 'Tous'], ['disponible', 'Disponibles'],
    ['en_service', 'En service'], ['maintenance', 'Maintenance'],
  ];

  poser(`
    <header class="entete">
      <h1>Gestion des véhicules</h1>
      <p class="sous-titre">Bonjour ${esc(u.prenom)} ${esc(u.nom)} — gérez vos véhicules de patrouille</p>
    </header>
    <div id="zone-message"></div>
    ${vieilleteInstantane}
    ${banniere}
    <div class="stats">
      ${tuile('', icones.vehicule, 'Total', stats.total, 'tous')}
      ${tuile('vert', icones.vehicule, 'Disponibles', stats.disponibles, 'disponible')}
      ${tuile('bleu', icones.vehicule, 'En service', stats.en_service, 'en_service')}
      ${tuile('ambre', icones.vehicule, 'Maintenance', stats.maintenance, 'maintenance')}
    </div>
    <div class="duo">
      <a class="btn" href="/scan">${icones.qr} Scanner</a>
      <a class="btn secondaire" href="/historique">${icones.historique} Historique</a>
    </div>
    ${stats.incidents_ouverts > 0 && session.peut('chef') ? `
      <a class="message attention" href="/incidents" style="display:block;text-decoration:none">
        ${stats.incidents_ouverts} incident${stats.incidents_ouverts > 1 ? 's' : ''} en attente de traitement
      </a>` : ''}
    <div class="filtres" role="tablist">
      ${filtres.map(([v, l]) => `
        <button class="filtre" role="tab" aria-selected="${filtreParc === v}"
                data-action="filtrer" data-statut="${v}">${l}</button>`).join('')}
    </div>
    ${session.peut('chef') ? `
      <a class="btn secondaire" href="/parc/nouveau" style="margin-bottom:14px">
        Ajouter un véhicule</a>` : ''}
    ${vehicules.length
      ? vehicules.map((v) => carteVehicule(v)).join('')
      : vide(filtreParc === 'tous'
          ? "Aucun véhicule enregistré pour le moment."
          : "Aucun véhicule dans cette catégorie.")}
  `);

  surClic(async (action, data) => {
    if (action === 'filtrer') {
      filtreParc = data.statut;
      await vueAccueil();
    } else if (action === 'prendre') {
      aller(`/prendre/${data.id}`);
    } else if (action === 'restituer') {
      aller(`/restituer/${data.checkout}`);
    } else if (action === 'fiche') {
      aller(`/vehicule/${data.id}`);
    }
  });
}

// --- Prise en compte -------------------------------------------------------

// Points de contrôle passés en revue au départ comme au retour. La liste est
// volontairement courte : un état des lieux qu'on ne peut pas remplir en
// trente secondes sur un parking ne sera pas rempli du tout.
const POINTS_CONTROLE = [
  ['carburant', 'Niveau de carburant suffisant'],
  ['proprete', 'Véhicule propre'],
  ['equipements', 'Équipements de bord présents'],
  ['documents', 'Carte grise et attestation d\'assurance'],
  ['carrosserie', 'Carrosserie sans dommage nouveau'],
];

function casesControle(prefixe) {
  return POINTS_CONTROLE.map(([cle, libelle]) => `
    <label class="case">
      <input type="checkbox" name="${prefixe}_${cle}" checked>
      <span>${esc(libelle)}</span>
    </label>`).join('');
}

function releverControle(form, prefixe) {
  const etat = {};
  for (const [cle] of POINTS_CONTROLE) {
    etat[cle] = form[`${prefixe}_${cle}`].checked;
  }
  return etat;
}

// vehiculeAffichable charge une fiche, en retombant sur l'instantané local
// lorsque le réseau manque. Sans ce repli, un agent hors ligne ne pourrait même
// pas ouvrir le formulaire de prise en compte.
async function vehiculeAffichable(id) {
  try {
    const d = await api.vehicule(id);
    return { vehicule: d.vehicule, complet: d, horsLigne: false };
  } catch (err) {
    if (!(err instanceof ErreurAPI) || err.statut !== 0) throw err;
    const v = parcMemorise()?.vehicules.find((x) => x.id === id);
    if (!v) return null;
    return { vehicule: v, complet: { vehicule: v }, horsLigne: true };
  }
}

async function vuePriseEnCompte(vehiculeId) {
  chargement();
  const charge = await vehiculeAffichable(vehiculeId);
  if (!charge) {
    poser(retour('/') + message('erreur',
      "Ce véhicule n'est pas connu de cet appareil et le réseau est indisponible."));
    return;
  }
  const v = charge.vehicule;
  const moi = session.utilisateur();

  // Un chef peut enregistrer la sortie au nom d'un équipage qui ne peut pas le
  // faire lui-même — téléphone oublié, saisie au poste avant le départ.
  // Impossible hors ligne : la liste des agents vient du serveur.
  let agents = [];
  if (session.peut('chef') && !charge.horsLigne) {
    try {
      agents = await api.agents();
    } catch {
      agents = []; // sans la liste, on retombe sur une saisie à son propre nom
    }
  }

  if (v.statut !== 'disponible') {
    poser(retour('/') + message('erreur',
      `Le véhicule ${v.code} n'est pas disponible (${LIBELLES_STATUT[v.statut] || v.statut}).`));
    return;
  }

  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <h1>Prise en compte</h1>
      <p class="sous-titre">${esc(v.code)} — ${esc([v.marque, v.modele].filter(Boolean).join(' '))}</p>
    </header>
    <div id="zone-message"></div>
    <form id="form-prise">
      <div class="carte">
        <div class="champ">
          <label for="km">Kilométrage au départ</label>
          <input type="number" id="km" name="km" inputmode="numeric" min="${v.km}"
                 value="${v.km}" required>
          <p class="aide">Compteur à la dernière restitution : ${nombre(v.km)} km.
            Relevez la valeur affichée au tableau de bord.</p>
        </div>
        <div class="champ">
          <label for="motif">Motif de la sortie</label>
          <input type="text" id="motif" name="motif"
                 placeholder="Patrouille secteur centre, transfert, formation…">
        </div>
        ${agents.length > 1 ? `
          <div class="champ" style="margin-bottom:0">
            <label for="agent">Véhicule confié à</label>
            <select id="agent" name="agent">
              ${agents.map((a) => `<option value="${a.id}"${a.id === moi.id ? ' selected' : ''}>
                ${esc(a.prenom)} ${esc(a.nom)} — ${esc(a.matricule)}${a.id === moi.id ? ' (vous)' : ''}
              </option>`).join('')}
            </select>
            <p class="aide">Pour enregistrer la sortie d'un équipage qui ne peut pas
              le faire lui-même. La saisie reste tracée à votre nom.</p>
          </div>` : ''}
      </div>

      <div class="carte">
        <h2 style="margin-bottom:8px">État des lieux au départ</h2>
        <p class="discret" style="margin-bottom:6px">Décochez ce qui pose problème.</p>
        ${casesControle('depart')}
        <div class="champ" style="margin-top:14px;margin-bottom:0">
          <label for="notes">Observations</label>
          <textarea id="notes" name="notes"
                    placeholder="Rayure portière avant droite, plein à faire…"></textarea>
        </div>
      </div>

      <button class="btn" type="submit">Confirmer la prise en compte</button>
    </form>`);

  const form = app.querySelector('#form-prise');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const bouton = form.querySelector('button[type=submit]');
    await enAttente(bouton, () => envoyerPrise(form, v, false));
  });
}

async function envoyerPrise(form, v, force) {
  const donnees = {
    vehicule_id: v.id,
    km: Number(form.km.value),
    motif: form.motif.value.trim(),
    notes: form.notes.value.trim(),
    check: releverControle(form, 'depart'),
    force,
  };
  if (form.agent) donnees.agent_id = Number(form.agent.value);

  try {
    const c = await api.prendreEnCompte({ ...donnees, cle_client: nouvelleCle() });
    await aller('/');
    const moi = session.utilisateur();
    poserMessage('succes', c.user_id === moi.id
      ? `${c.vehicle_code} pris en compte à ${nombre(c.km_start)} km. Bonne patrouille.`
      : `${c.vehicle_code} confié à ${c.user_nom} à ${nombre(c.km_start)} km.`);
  } catch (err) {
    // Réseau absent : la sortie est enregistrée sur l'appareil et transmise
    // plus tard. Un agent en sous-sol ne doit pas être empêché de partir.
    if (err instanceof ErreurAPI && err.statut === 0) {
      return misEnAttentePrise(donnees, v);
    }
    // Le serveur refuse un écart de kilométrage aberrant. C'est presque
    // toujours une faute de frappe, mais parfois le compteur a réellement
    // bougé (véhicule déplacé par le garage) : on laisse confirmer.
    if (err.code === 'km_incoherent' && !force
        && confirm(`${err.message}\n\nConfirmez-vous cette valeur ?`)) {
      return envoyerPrise(form, v, true);
    }
    poserMessage('erreur', err.message);
  }
}

async function misEnAttentePrise(donnees, v) {
  const maintenant = new Date().toISOString();
  empiler('prise', {
    ...donnees,
    cle_client: nouvelleCle(),
    date_debut: maintenant,
    hors_ligne: true,
  }, `${v.code} pris en compte à ${nombre(donnees.km)} km`);

  // L'instantané local reflète la sortie : sans cela, l'écran d'accueil
  // proposerait encore le véhicule comme disponible.
  const parc = parcMemorise();
  if (parc) {
    const cible = parc.vehicules.find((x) => x.id === v.id);
    if (cible) {
      const u = session.utilisateur();
      cible.statut = 'en_service';
      cible.km = donnees.km;
      cible.checkout_en_cours = {
        id: 0, user_id: u.id, user_nom: `${u.prenom} ${u.nom}`,
        started_at: maintenant, km_start: donnees.km, en_attente: true,
      };
    }
    memoriserParc(parc.vehicules, parc.stats);
  }

  await aller('/');
  poserMessage('attention',
    `${v.code} pris en compte hors ligne. L'enregistrement sera transmis au retour du réseau.`);
}

// --- Restitution -----------------------------------------------------------

async function vueRestitution(checkoutId) {
  chargement();

  let c;
  try {
    const prises = await api.prises({ statut: 'en_cours', limite: 200 });
    c = prises.find((p) => p.id === checkoutId);
  } catch (err) {
    if (!(err instanceof ErreurAPI) || err.statut !== 0) throw err;
    // Hors ligne : la sortie en cours figure dans l'instantané du parc.
    const parc = parcMemorise();
    c = parc?.vehicules
      .map((v) => v.checkout_en_cours && { ...v.checkout_en_cours, vehicle_id: v.id, vehicle_code: v.code })
      .find((x) => x && x.id === checkoutId);
  }

  if (!c) {
    poser(retour('/') + message('erreur',
      "Cette prise en compte est introuvable ou déjà clôturée."));
    return;
  }

  const u = session.utilisateur();
  const pourAutrui = c.user_id !== u.id;

  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <h1>Restitution</h1>
      <p class="sous-titre">${esc(c.vehicle_code)} — pris à ${nombre(c.km_start)} km
        le ${dateHeure(c.started_at)}</p>
    </header>
    <div id="zone-message"></div>
    ${pourAutrui ? message('attention',
      `Cette sortie est au nom de ${c.user_nom}. La clôture sera enregistrée à votre nom dans le journal.`) : ''}
    <form id="form-retour">
      <div class="carte">
        <div class="champ">
          <label for="km">Kilométrage au retour</label>
          <input type="number" id="km" name="km" inputmode="numeric"
                 min="${c.km_start}" value="${c.km_start}" required>
          <p class="aide" id="distance">Kilométrage au départ : ${nombre(c.km_start)} km.</p>
        </div>
      </div>

      <div class="carte">
        <h2 style="margin-bottom:8px">État des lieux au retour</h2>
        <p class="discret" style="margin-bottom:6px">Décochez ce qui pose problème.</p>
        ${casesControle('retour')}
        <div class="champ" style="margin-top:14px;margin-bottom:0">
          <label for="notes">Observations</label>
          <textarea id="notes" name="notes" placeholder="RAS"></textarea>
        </div>
      </div>

      <div class="carte">
        <h2 style="margin-bottom:10px">Signaler un incident</h2>
        <label class="case">
          <input type="checkbox" id="avec-incident">
          <span>Un incident est survenu pendant la sortie</span>
        </label>
        <div id="bloc-incident" hidden style="margin-top:12px">
          <div class="grille-2">
            <div class="champ">
              <label for="inc-type">Nature</label>
              <select id="inc-type">
                ${Object.entries(LIBELLES_INCIDENT).map(([v, l]) =>
                  `<option value="${v}">${l}</option>`).join('')}
              </select>
            </div>
            <div class="champ">
              <label for="inc-gravite">Gravité</label>
              <select id="inc-gravite">
                ${Object.entries(LIBELLES_GRAVITE).map(([v, l]) =>
                  `<option value="${v}">${l}</option>`).join('')}
              </select>
            </div>
          </div>
          <div class="champ" style="margin-bottom:0">
            <label for="inc-description">Description</label>
            <textarea id="inc-description"
                      placeholder="Pare-chocs arrière enfoncé sur un plot, sans tiers impliqué."></textarea>
            <p class="aide">Un incident « immobilisant » place automatiquement le
              véhicule en maintenance : il ne pourra plus être pris en compte.</p>
          </div>
        </div>
      </div>

      <button class="btn" type="submit">Confirmer la restitution</button>
    </form>`);

  const form = app.querySelector('#form-retour');
  const distance = app.querySelector('#distance');
  form.km.addEventListener('input', () => {
    const parcourus = Number(form.km.value) - c.km_start;
    distance.textContent = Number.isFinite(parcourus) && parcourus >= 0
      ? `${nombre(parcourus)} km parcourus (départ : ${nombre(c.km_start)} km).`
      : `Kilométrage au départ : ${nombre(c.km_start)} km.`;
  });

  const caseIncident = app.querySelector('#avec-incident');
  const blocIncident = app.querySelector('#bloc-incident');
  caseIncident.addEventListener('change', () => {
    blocIncident.hidden = !caseIncident.checked;
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const bouton = form.querySelector('button[type=submit]');
    await enAttente(bouton, () => envoyerRetour(form, c, false));
  });
}

async function envoyerRetour(form, c, force) {
  const incidents = [];
  if (app.querySelector('#avec-incident').checked) {
    const description = app.querySelector('#inc-description').value.trim();
    if (!description) {
      poserMessage('erreur', "Décrivez l'incident, ou décochez la case.");
      return;
    }
    incidents.push({
      type: app.querySelector('#inc-type').value,
      gravite: app.querySelector('#inc-gravite').value,
      description,
    });
  }

  const corps = {
    km: Number(form.km.value),
    notes: form.notes.value.trim(),
    check: releverControle(form, 'retour'),
    incidents,
    force,
  };

  // Une sortie encore en attente de transmission n'a pas d'identifiant côté
  // serveur : impossible de la clôturer tant qu'elle n'est pas partie.
  if (!c.id) {
    poserMessage('erreur',
      "La prise en compte de ce véhicule n'a pas encore été transmise. "
      + "Reconnectez-vous au réseau avant de le restituer.");
    return;
  }

  try {
    const fin = await api.restituer(c.id, { ...corps, cle_client: nouvelleCle() });
    await aller('/');
    poserMessage('succes',
      `${fin.vehicle_code} restitué — ${nombre(fin.distance_km)} km parcourus.`);
  } catch (err) {
    if (err instanceof ErreurAPI && err.statut === 0) {
      return misEnAttenteRetour(corps, c);
    }
    if (err.code === 'km_incoherent' && !force
        && confirm(`${err.message}\n\nConfirmez-vous cette valeur ?`)) {
      return envoyerRetour(form, c, true);
    }
    poserMessage('erreur', err.message);
  }
}

async function misEnAttenteRetour(corps, c) {
  const maintenant = new Date().toISOString();
  empiler('restitution', {
    checkout_id: c.id,
    corps: { ...corps, cle_client: nouvelleCle(), date_retour: maintenant, hors_ligne: true },
  }, `${c.vehicle_code} restitué à ${nombre(corps.km)} km`);

  const parc = parcMemorise();
  if (parc) {
    const cible = parc.vehicules.find((x) => x.id === c.vehicle_id);
    if (cible) {
      cible.statut = corps.incidents.some((i) => i.gravite === 'immobilisant')
        ? 'maintenance' : 'disponible';
      cible.km = corps.km;
      delete cible.checkout_en_cours;
    }
    memoriserParc(parc.vehicules, parc.stats);
  }

  await aller('/');
  poserMessage('attention',
    `${c.vehicle_code} restitué hors ligne. L'enregistrement sera transmis au retour du réseau.`);
}

// --- Scan du QR code -------------------------------------------------------

let fluxCamera = null;

function arreterCamera() {
  if (fluxCamera) {
    fluxCamera.getTracks().forEach((p) => p.stop());
    fluxCamera = null;
  }
}
window.addEventListener('popstate', arreterCamera);

async function vueScan(jetonDirect) {
  // Cas d'un QR ouvert depuis l'appareil photo du téléphone : l'adresse
  // contient déjà le jeton, il n'y a rien à scanner.
  if (jetonDirect) {
    chargement();
    try {
      const { vehicule } = await api.scanner(jetonDirect);
      return aller(`/vehicule/${vehicule.id}`, true);
    } catch (err) {
      poser(retour('/') + message('erreur', err.message));
      return;
    }
  }

  const supporte = 'BarcodeDetector' in window;
  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <h1>Scanner un véhicule</h1>
      <p class="sous-titre">Visez le QR code collé dans le véhicule</p>
    </header>
    <div id="zone-message"></div>
    ${supporte ? `
      <div class="carte">
        <video id="scan-video" playsinline muted></video>
      </div>` : message('info',
        "Votre navigateur ne sait pas lire les QR codes. Saisissez le code du véhicule ci-dessous.")}
    <div class="carte">
      <form id="form-code">
        <div class="champ" style="margin-bottom:12px">
          <label for="code">Code du véhicule</label>
          <input type="text" id="code" name="code" placeholder="TV1"
                 autocapitalize="characters" spellcheck="false">
          <p class="aide">Saisie manuelle, si le QR code est illisible ou décollé.</p>
        </div>
        <button class="btn secondaire" type="submit">Ouvrir la fiche</button>
      </form>
    </div>`);

  app.querySelector('#form-code').addEventListener('submit', async (e) => {
    e.preventDefault();
    const code = app.querySelector('#code').value.trim();
    if (!code) return;
    const vehicules = await api.vehicules('tous');
    const v = vehicules.find((x) => x.code.toLowerCase() === code.toLowerCase());
    if (!v) {
      poserMessage('erreur', `Aucun véhicule ne porte le code « ${code} ».`);
      return;
    }
    arreterCamera();
    aller(`/vehicule/${v.id}`);
  });

  if (supporte) demarrerScan();
}

async function demarrerScan() {
  const video = app.querySelector('#scan-video');
  if (!video) return;

  try {
    fluxCamera = await navigator.mediaDevices.getUserMedia({
      video: { facingMode: 'environment' },
    });
  } catch {
    poserMessage('info',
      "Accès à la caméra refusé. Utilisez la saisie manuelle ci-dessous.");
    return;
  }
  video.srcObject = fluxCamera;
  await video.play().catch(() => {});

  const detecteur = new BarcodeDetector({ formats: ['qr_code'] });
  let arrete = false;

  const boucle = async () => {
    if (arrete || !document.body.contains(video)) {
      arreterCamera();
      return;
    }
    try {
      const codes = await detecteur.detect(video);
      if (codes.length) {
        arrete = true;
        arreterCamera();
        await ouvrirDepuisQR(codes[0].rawValue);
        return;
      }
    } catch {
      // Image non exploitable (mise au point en cours) : on réessaie.
    }
    requestAnimationFrame(boucle);
  };
  requestAnimationFrame(boucle);
}

async function ouvrirDepuisQR(valeur) {
  // Le QR contient soit une URL complète, soit « vlpm:<jeton> » quand le
  // serveur n'a pas d'URL publique configurée.
  const jeton = String(valeur).replace(/^vlpm:/, '').split('/').filter(Boolean).pop();
  try {
    const { vehicule } = await api.scanner(jeton);
    aller(`/vehicule/${vehicule.id}`);
  } catch (err) {
    poserMessage('erreur', err.message);
  }
}

// --- Fiche véhicule --------------------------------------------------------

async function vueFicheVehicule(id) {
  chargement();
  const charge = await vehiculeAffichable(id);
  if (!charge) {
    poser(retour('/') + message('erreur',
      "Ce véhicule n'est pas connu de cet appareil et le réseau est indisponible."));
    return;
  }
  const d = charge.complet;
  const v = charge.vehicule;
  const enCours = d.checkout_en_cours || v.checkout_en_cours;
  // Hors ligne, seules les caractéristiques mémorisées sont disponibles :
  // l'historique, les incidents et les entretiens exigent le serveur.
  const chef = session.peut('chef') && !charge.horsLigne;

  const ligne = (cle, val) =>
    `<div class="champ-lecture"><div class="cle">${esc(cle)}</div><div class="val">${val}</div></div>`;

  const incidentsOuverts = (d.incidents || []).filter((i) => i.statut !== 'resolu');

  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <div class="entre-deux">
        <div><h1>${esc(v.code)}</h1>
          <p class="sous-titre">${esc([v.marque, v.modele].filter(Boolean).join(' ')) || 'Véhicule'}</p></div>
        ${badge(v.statut)}
      </div>
    </header>
    <div id="zone-message"></div>

    ${enCours ? `
      <div class="carte" style="border-color:var(--marine)">
        <div class="pile" style="margin-bottom:12px">
          <strong>En service — ${esc(enCours.user_nom)}</strong>
          <span class="discret">Depuis ${dateHeure(enCours.started_at)},
            ${nombre(enCours.km_start)} km au départ</span>
        </div>
        <button class="btn" data-action="restituer" data-checkout="${enCours.id}">
          ${icones.sortie} Restituer</button>
      </div>` : ''}

    ${v.statut === 'disponible' ? `
      <button class="btn" data-action="prendre" data-id="${v.id}" style="margin-bottom:14px">
        ${icones.qr} Prendre en compte</button>` : ''}

    ${incidentsOuverts.length ? `
      <div class="carte">
        <h2 style="margin-bottom:12px">Incidents non résolus</h2>
        ${incidentsOuverts.map((i) => `
          <div class="champ-lecture">
            <div class="entre-deux" style="margin-bottom:5px">
              <strong>${esc(LIBELLES_INCIDENT[i.type] || i.type)}</strong>
              <span class="badge ${i.gravite === 'immobilisant' ? 'hors_service' : 'maintenance'}">
                ${esc(LIBELLES_GRAVITE[i.gravite] || i.gravite)}</span>
            </div>
            <div class="val" style="font-weight:400">${esc(i.description)}</div>
            <div class="discret" style="margin-top:5px">
              Signalé par ${esc(i.user_nom)} le ${dateHeure(i.created_at)}</div>
            ${chef ? `<button class="btn secondaire compact" style="margin-top:9px"
              data-action="resoudre" data-id="${i.id}">Marquer comme résolu</button>` : ''}
          </div>`).join('')}
      </div>` : ''}

    <div class="carte">
      <h2 style="margin-bottom:8px">Caractéristiques</h2>
      ${ligne('Kilométrage', `${nombre(v.km)} km`)}
      ${v.immatriculation ? ligne('Immatriculation', esc(v.immatriculation)) : ''}
      ${v.date_mise_circulation ? ligne('Mise en circulation', dateCourte(v.date_mise_circulation)) : ''}
      ${v.prochain_ct ? ligne('Prochain contrôle technique', dateCourte(v.prochain_ct)) : ''}
      ${v.prochaine_revision_km ? ligne('Prochaine révision', `${nombre(v.prochaine_revision_km)} km`) : ''}
      ${v.notes ? ligne('Notes', esc(v.notes)) : ''}
    </div>

    ${chef ? `
      <div class="duo">
        <a class="btn secondaire" href="/vehicule/${v.id}/modifier">Modifier</a>
        <button class="btn secondaire" data-action="qrcode" data-id="${v.id}">
          ${icones.qr} Étiquette QR</button>
      </div>
      <div class="carte" id="bloc-qr" hidden></div>
      <div class="carte">
        <h2 style="margin-bottom:12px">Entretiens</h2>
        ${(d.maintenances || []).length ? (d.maintenances).map((m) => `
          <div class="champ-lecture">
            <div class="entre-deux">
              <strong>${esc(LIBELLES_ENTRETIEN[m.type] || m.type)}</strong>
              <span class="discret">${dateCourte(m.date)}</span>
            </div>
            <div class="discret">${esc(m.description) || '—'}
              ${m.cout_cents ? ` — ${euros(m.cout_cents)}` : ''}
              ${m.prestataire ? ` — ${esc(m.prestataire)}` : ''}</div>
          </div>`).join('') : '<p class="discret">Aucun entretien enregistré.</p>'}
        <button class="btn secondaire compact" style="margin-top:12px"
                data-action="ajout-entretien" data-id="${v.id}">Ajouter un entretien</button>
      </div>` : ''}

    <div class="carte">
      <h2 style="margin-bottom:12px">Dernières sorties</h2>
      ${(d.historique || []).length
        ? d.historique.map(ligneHistorique).join('')
        : '<p class="discret">Aucune sortie enregistrée.</p>'}
    </div>`);

  surClic(async (action, data, e, cible) => {
    if (action === 'prendre') aller(`/prendre/${data.id}`);
    else if (action === 'restituer') aller(`/restituer/${data.checkout}`);
    else if (action === 'qrcode') afficherQR(data.id, v.code);
    else if (action === 'ajout-entretien') formulaireEntretien(v);
    else if (action === 'resoudre') {
      await enAttente(cible, async () => {
        try {
          await api.resoudreIncident(Number(data.id));
          await vueFicheVehicule(id);
          poserMessage('succes', 'Incident marqué comme résolu.');
        } catch (err) {
          poserMessage('erreur', err.message);
        }
      });
    }
  });
}

function ligneHistorique(c) {
  const distance = c.distance_km != null
    ? `<span class="gain">+ ${nombre(c.distance_km)} km parcourus</span>` : '';
  return `
    <div class="champ-lecture">
      <div class="entre-deux" style="margin-bottom:5px">
        <strong>${esc(c.vehicle_code)}</strong>
        ${badge(c.statut, { en_cours: 'En cours', termine: 'Terminé' })}
      </div>
      <div class="discret">${esc(c.user_nom)}${c.saisi_par ? ' (sortie saisie au poste)' : ''}</div>
      <div class="discret">Départ ${dateHeure(c.started_at)} — ${nombre(c.km_start)} km</div>
      ${c.ended_at
        ? `<div class="discret">Retour ${dateHeure(c.ended_at)} — ${nombre(c.km_end)} km</div>${distance}`
        : ''}
      ${c.motif ? `<div class="discret">Motif : ${esc(c.motif)}</div>` : ''}
      ${c.notes_retour ? `<div class="discret">Retour : ${esc(c.notes_retour)}</div>` : ''}
    </div>`;
}

async function afficherQR(id, code) {
  const bloc = app.querySelector('#bloc-qr');
  if (!bloc) return;
  if (!bloc.hidden) { bloc.hidden = true; return; }

  // L'image passe par fetch pour porter l'en-tête d'authentification, puis est
  // injectée en blob : une balise <img src> nue serait refusée par l'API.
  bloc.hidden = false;
  bloc.innerHTML = '<p class="discret">Génération de l\'étiquette…</p>';
  try {
    const rep = await fetch(`/api/v1/vehicules/${id}/qrcode.png?taille=512`, {
      headers: { Authorization: `Bearer ${session.jeton()}` },
    });
    if (!rep.ok) throw new Error('Étiquette indisponible.');
    const url = URL.createObjectURL(await rep.blob());
    bloc.innerHTML = `
      <h2 style="margin-bottom:12px">Étiquette ${esc(code)}</h2>
      <img class="qr-apercu" src="${url}" alt="QR code du véhicule ${esc(code)}">
      <p class="discret" style="text-align:center;margin-bottom:12px">
        Imprimez cette étiquette et collez-la dans le véhicule.</p>
      <a class="btn secondaire" href="${url}" download="qr-${esc(code)}.png">Télécharger</a>`;
  } catch (err) {
    bloc.innerHTML = message('erreur', err.message);
  }
}

function formulaireEntretien(v) {
  poser(`
    ${retour(`/vehicule/${v.id}`, `Retour à la fiche ${v.code}`)}
    <header class="entete"><h1>Nouvel entretien</h1>
      <p class="sous-titre">${esc(v.code)}</p></header>
    <div id="zone-message"></div>
    <form id="form-entretien">
      <div class="carte">
        <div class="grille-2">
          <div class="champ">
            <label for="type">Nature</label>
            <select id="type" name="type">
              ${Object.entries(LIBELLES_ENTRETIEN).map(([val, l]) =>
                `<option value="${val}">${l}</option>`).join('')}
            </select>
          </div>
          <div class="champ">
            <label for="date">Date</label>
            <input type="date" id="date" name="date" value="${new Date().toISOString().slice(0, 10)}">
          </div>
        </div>
        <div class="grille-2">
          <div class="champ">
            <label for="km">Kilométrage</label>
            <input type="number" id="km" name="km" inputmode="numeric" value="${v.km}">
          </div>
          <div class="champ">
            <label for="cout">Coût (€)</label>
            <input type="text" id="cout" name="cout" inputmode="decimal" placeholder="149,90">
          </div>
        </div>
        <div class="champ">
          <label for="prestataire">Prestataire</label>
          <input type="text" id="prestataire" name="prestataire" placeholder="Garage municipal">
        </div>
        <div class="champ" style="margin-bottom:0">
          <label for="description">Description</label>
          <textarea id="description" name="description"></textarea>
        </div>
      </div>
      ${v.statut === 'maintenance' ? `
        <div class="carte">
          <label class="case">
            <input type="checkbox" name="remettre" checked>
            <span>Remettre le véhicule en service après cet entretien</span>
          </label>
        </div>` : ''}
      <button class="btn" type="submit">Enregistrer l'entretien</button>
    </form>`);

  const form = app.querySelector('#form-entretien');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await enAttente(form.querySelector('button[type=submit]'), async () => {
      try {
        await api.creerEntretien({
          vehicule_id: v.id,
          type: form.type.value,
          date: form.date.value,
          km: form.km.value ? Number(form.km.value) : null,
          cout_euros: form.cout.value.trim(),
          prestataire: form.prestataire.value.trim(),
          description: form.description.value.trim(),
          remettre_en_service: Boolean(form.remettre?.checked),
        });
        await aller(`/vehicule/${v.id}`);
        poserMessage('succes', 'Entretien enregistré.');
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });
}

// --- Création et modification d'un véhicule --------------------------------

async function vueFormVehicule(id) {
  const nouveau = id === null;
  let v = {
    code: '', marque: '', modele: '', immatriculation: '', km: 0,
    statut: 'disponible', date_mise_circulation: '', prochain_ct: '',
    prochaine_revision_km: '', notes: '', archive: false,
  };
  if (!nouveau) {
    chargement();
    v = (await api.vehicule(id)).vehicule;
  }

  // « En service » découle des prises en compte : il n'est pas proposé ici.
  const statutsModifiables = ['disponible', 'maintenance', 'hors_service'];

  poser(`
    ${retour(nouveau ? '/' : `/vehicule/${id}`, 'Retour')}
    <header class="entete"><h1>${nouveau ? 'Ajouter un véhicule' : `Modifier ${esc(v.code)}`}</h1></header>
    <div id="zone-message"></div>
    <form id="form-vehicule">
      <div class="carte">
        <div class="grille-2">
          <div class="champ">
            <label for="code">Code *</label>
            <input type="text" id="code" name="code" value="${esc(v.code)}"
                   placeholder="TV1" autocapitalize="characters" required>
          </div>
          <div class="champ">
            <label for="immatriculation">Immatriculation</label>
            <input type="text" id="immatriculation" name="immatriculation"
                   value="${esc(v.immatriculation)}" placeholder="AB-123-CD"
                   autocapitalize="characters">
          </div>
        </div>
        <div class="grille-2">
          <div class="champ">
            <label for="marque">Marque</label>
            <input type="text" id="marque" name="marque" value="${esc(v.marque)}">
          </div>
          <div class="champ">
            <label for="modele">Modèle</label>
            <input type="text" id="modele" name="modele" value="${esc(v.modele)}">
          </div>
        </div>
        <div class="grille-2">
          <div class="champ">
            <label for="km">Kilométrage</label>
            <input type="number" id="km" name="km" inputmode="numeric" value="${v.km}">
          </div>
          <div class="champ">
            <label for="statut">Statut</label>
            <select id="statut" name="statut" ${v.statut === 'en_service' ? 'disabled' : ''}>
              ${statutsModifiables.map((s) =>
                `<option value="${s}"${v.statut === s ? ' selected' : ''}>${LIBELLES_STATUT[s]}</option>`).join('')}
            </select>
            ${v.statut === 'en_service'
              ? '<p class="aide">Véhicule en service : clôturez la sortie pour changer son statut.</p>'
              : ''}
          </div>
        </div>
      </div>

      <div class="carte">
        <h2 style="margin-bottom:12px">Suivi</h2>
        <div class="grille-2">
          <div class="champ">
            <label for="date_mise_circulation">Mise en circulation</label>
            <input type="date" id="date_mise_circulation" name="date_mise_circulation"
                   value="${esc((v.date_mise_circulation || '').slice(0, 10))}">
          </div>
          <div class="champ">
            <label for="prochain_ct">Prochain contrôle technique</label>
            <input type="date" id="prochain_ct" name="prochain_ct"
                   value="${esc((v.prochain_ct || '').slice(0, 10))}">
          </div>
        </div>
        <div class="champ">
          <label for="prochaine_revision_km">Prochaine révision (km)</label>
          <input type="number" id="prochaine_revision_km" name="prochaine_revision_km"
                 inputmode="numeric" value="${v.prochaine_revision_km ?? ''}">
        </div>
        <div class="champ" style="margin-bottom:0">
          <label for="notes">Notes</label>
          <textarea id="notes" name="notes">${esc(v.notes)}</textarea>
        </div>
      </div>

      <button class="btn" type="submit">${nouveau ? 'Créer le véhicule' : 'Enregistrer'}</button>
    </form>`);

  const form = app.querySelector('#form-vehicule');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await enAttente(form.querySelector('button[type=submit]'), async () => {
      const donnees = {
        code: form.code.value.trim(),
        marque: form.marque.value.trim(),
        modele: form.modele.value.trim(),
        immatriculation: form.immatriculation.value.trim(),
        km: Number(form.km.value) || 0,
        date_mise_circulation: form.date_mise_circulation.value,
        prochain_ct: form.prochain_ct.value,
        prochaine_revision_km: form.prochaine_revision_km.value
          ? Number(form.prochaine_revision_km.value) : null,
        notes: form.notes.value.trim(),
      };
      if (!form.statut.disabled) donnees.statut = form.statut.value;

      try {
        if (nouveau) {
          const cree = await api.creerVehicule(donnees);
          await aller(`/vehicule/${cree.id}`);
          poserMessage('succes',
            `${cree.code} ajouté au parc. Imprimez son étiquette QR depuis sa fiche.`);
        } else {
          await api.modifierVehicule(id, donnees);
          await aller(`/vehicule/${id}`);
          poserMessage('succes', 'Véhicule mis à jour.');
        }
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });
}

// --- Historique ------------------------------------------------------------

let filtreHistorique = 'tous';

async function vueHistorique() {
  chargement();
  const prises = await api.prises({
    statut: filtreHistorique === 'tous' ? '' : filtreHistorique,
    limite: 200,
  });
  const chef = session.peut('chef');

  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <h1>Historique des prises en compte</h1>
      <p class="sous-titre">${chef
        ? "Consultez l'historique complet des utilisations de véhicules"
        : "Vos sorties de véhicules"}</p>
    </header>
    <div id="zone-message"></div>
    <div class="filtres" role="tablist">
      ${[['tous', 'Tous'], ['en_cours', 'En cours'], ['termine', 'Terminés']]
        .map(([v, l]) => `<button class="filtre" role="tab"
          aria-selected="${filtreHistorique === v}"
          data-action="filtrer" data-statut="${v}">${l}</button>`).join('')}
    </div>
    ${chef ? `<a class="btn secondaire" style="margin-bottom:14px"
      href="/api/v1/export/prises.csv" data-action="export">Exporter en CSV</a>` : ''}
    ${prises.length
      ? prises.map((c) => `<article class="carte">${ligneHistorique(c)}</article>`).join('')
      : vide("Aucune prise en compte enregistrée.", icones.historique)}`);

  surClic(async (action, data, e) => {
    if (action === 'filtrer') {
      filtreHistorique = data.statut;
      await vueHistorique();
    } else if (action === 'export') {
      // Le téléchargement doit porter l'en-tête d'authentification : on passe
      // donc par fetch plutôt que de laisser le navigateur suivre le lien.
      e.preventDefault();
      await telecharger('/api/v1/export/prises.csv', 'vlpm-historique.csv');
    }
  });
}

async function telecharger(url, nomFichier) {
  try {
    const rep = await fetch(url, {
      headers: { Authorization: `Bearer ${session.jeton()}` },
    });
    if (!rep.ok) throw new Error("L'export a échoué.");
    const lien = document.createElement('a');
    lien.href = URL.createObjectURL(await rep.blob());
    lien.download = nomFichier;
    lien.click();
    URL.revokeObjectURL(lien.href);
  } catch (err) {
    poserMessage('erreur', err.message);
  }
}

// --- Incidents -------------------------------------------------------------

async function vueIncidents() {
  chargement();
  const incidents = await api.incidents({ statut: 'ouverts' });

  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <h1>Incidents</h1>
      <p class="sous-titre">Signalements en attente de traitement</p>
    </header>
    <div id="zone-message"></div>
    ${incidents.length ? incidents.map((i) => `
      <article class="carte">
        <div class="entre-deux" style="margin-bottom:8px">
          <div class="pile">
            <strong>${esc(i.vehicle_code)} — ${esc(LIBELLES_INCIDENT[i.type] || i.type)}</strong>
            <span class="discret">${esc(i.user_nom)}, ${dateHeure(i.created_at)}</span>
          </div>
          <span class="badge ${i.gravite === 'immobilisant' ? 'hors_service' : 'maintenance'}">
            ${esc(LIBELLES_GRAVITE[i.gravite] || i.gravite)}</span>
        </div>
        <p style="margin-bottom:12px">${esc(i.description)}</p>
        <div class="duo" style="margin-bottom:0">
          <a class="btn secondaire compact" href="/vehicule/${i.vehicle_id}"
             style="width:100%">Voir le véhicule</a>
          <button class="btn compact" data-action="resoudre" data-id="${i.id}"
                  style="width:100%">Marquer résolu</button>
        </div>
      </article>`).join('')
      : vide("Aucun incident en attente. Le parc est en ordre.", icones.alerte)}`);

  surClic(async (action, data, e, cible) => {
    if (action !== 'resoudre') return;
    await enAttente(cible, async () => {
      try {
        await api.resoudreIncident(Number(data.id));
        await vueIncidents();
        poserMessage('succes', 'Incident marqué comme résolu.');
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });
}

// --- Agents ----------------------------------------------------------------

const LIBELLES_ROLE = { agent: 'Agent', chef: 'Chef de service', admin: 'Administrateur' };

async function vueAgents() {
  if (!session.peut('chef')) return vue404();
  chargement();
  const agents = await api.agents(true);
  const moi = session.utilisateur();

  poser(`
    ${retour('/reglages', 'Retour aux réglages')}
    <header class="entete">
      <h1>Agents</h1>
      <p class="sous-titre">Comptes autorisés à utiliser l'application</p>
    </header>
    <div id="zone-message"></div>
    <button class="btn" data-action="nouveau" style="margin-bottom:16px">Créer un compte</button>
    ${agents.map((a) => `
      <article class="carte">
        <div class="entre-deux">
          <div class="pile">
            <strong>${esc(a.prenom)} ${esc(a.nom)}</strong>
            <span class="discret">Matricule ${esc(a.matricule)} — ${esc(LIBELLES_ROLE[a.role] || a.role)}</span>
            ${a.derniere_connexion
              ? `<span class="discret">Dernière connexion ${dateHeure(a.derniere_connexion)}</span>`
              : '<span class="discret">Jamais connecté</span>'}
          </div>
          <span class="badge ${a.actif ? 'disponible' : 'hors_service'}">
            ${a.actif ? 'Actif' : 'Désactivé'}</span>
        </div>
        <div class="duo" style="margin-top:12px;margin-bottom:0">
          <button class="btn secondaire compact" style="width:100%"
                  data-action="mot-de-passe" data-id="${a.id}">Réinitialiser le mot de passe</button>
          ${a.id === moi.id ? '' : `
            <button class="btn secondaire compact" style="width:100%"
                    data-action="basculer" data-id="${a.id}" data-actif="${a.actif}">
              ${a.actif ? 'Désactiver' : 'Réactiver'}</button>`}
        </div>
      </article>`).join('')}`);

  surClic(async (action, data, e, cible) => {
    if (action === 'nouveau') return formulaireAgent();
    if (action === 'mot-de-passe') {
      if (!confirm("Réinitialiser le mot de passe ? Les sessions ouvertes de cet agent seront fermées.")) return;
      await enAttente(cible, async () => {
        try {
          const rep = await api.reinitialiserMotDePasse(Number(data.id));
          afficherMotDePasseProvisoire(rep.mot_de_passe_provisoire);
        } catch (err) {
          poserMessage('erreur', err.message);
        }
      });
    } else if (action === 'basculer') {
      const actif = data.actif === 'true';
      if (!confirm(actif
        ? "Désactiver ce compte ? L'agent ne pourra plus se connecter."
        : "Réactiver ce compte ?")) return;
      await enAttente(cible, async () => {
        try {
          await api.modifierAgent(Number(data.id), { actif: !actif });
          await vueAgents();
          poserMessage('succes', actif ? 'Compte désactivé.' : 'Compte réactivé.');
        } catch (err) {
          poserMessage('erreur', err.message);
        }
      });
    }
  });
}

function afficherMotDePasseProvisoire(mdp) {
  poser(`
    ${retour('/agents', 'Retour aux agents')}
    <header class="entete"><h1>Mot de passe provisoire</h1></header>
    <div class="carte">
      ${message('attention', "Ce mot de passe ne sera plus affiché. Notez-le et transmettez-le à l'agent.")}
      <div class="mdp-provisoire">${esc(mdp)}</div>
      <p class="discret">L'agent devra le changer à sa première connexion.</p>
    </div>
    <a class="btn" href="/agents">Terminé</a>`);
}

function formulaireAgent() {
  const admin = session.peut('admin');
  poser(`
    ${retour('/agents', 'Retour aux agents')}
    <header class="entete"><h1>Créer un compte</h1>
      <p class="sous-titre">Un mot de passe provisoire sera généré</p></header>
    <div id="zone-message"></div>
    <form id="form-agent">
      <div class="carte">
        <div class="champ">
          <label for="matricule">Matricule *</label>
          <input type="text" id="matricule" name="matricule" required
                 autocapitalize="none" spellcheck="false">
        </div>
        <div class="grille-2">
          <div class="champ">
            <label for="prenom">Prénom</label>
            <input type="text" id="prenom" name="prenom">
          </div>
          <div class="champ">
            <label for="nom">Nom *</label>
            <input type="text" id="nom" name="nom" required>
          </div>
        </div>
        <div class="champ">
          <label for="email">Courriel</label>
          <input type="email" id="email" name="email" autocapitalize="none">
        </div>
        <div class="champ" style="margin-bottom:0">
          <label for="role">Rôle</label>
          <select id="role" name="role">
            <option value="agent">Agent — prise en compte et restitution</option>
            <option value="chef">Chef de service — gestion du parc et des agents</option>
            ${admin ? '<option value="admin">Administrateur — accès complet</option>' : ''}
          </select>
        </div>
      </div>
      <button class="btn" type="submit">Créer le compte</button>
    </form>`);

  const form = app.querySelector('#form-agent');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await enAttente(form.querySelector('button[type=submit]'), async () => {
      try {
        const rep = await api.creerAgent({
          matricule: form.matricule.value.trim(),
          nom: form.nom.value.trim(),
          prenom: form.prenom.value.trim(),
          email: form.email.value.trim(),
          role: form.role.value,
        });
        afficherMotDePasseProvisoire(rep.mot_de_passe_provisoire);
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });
}

// --- Journal d'audit -------------------------------------------------------

const LIBELLES_ACTION = {
  login: 'Connexion', logout: 'Déconnexion',
  login_echec: 'Échec de connexion', login_bloque: 'Connexion bloquée',
  prise_en_compte: 'Prise en compte', restitution: 'Restitution',
  creation_vehicule: 'Création de véhicule', modification_vehicule: 'Modification de véhicule',
  creation_compte: 'Création de compte', modification_compte: 'Modification de compte',
  changement_mot_de_passe: 'Changement de mot de passe',
  reinitialisation_mot_de_passe: 'Réinitialisation de mot de passe',
  signalement_incident: 'Signalement d\'incident', resolution_incident: 'Résolution d\'incident',
  entretien: 'Entretien',
};

async function vueJournal() {
  if (!session.peut('admin')) return vue404();
  chargement();
  const entrees = await api.journal();

  poser(`
    ${retour('/reglages', 'Retour aux réglages')}
    <header class="entete">
      <h1>Journal d'activité</h1>
      <p class="sous-titre">200 dernières actions enregistrées</p>
    </header>
    <div class="carte">
      ${entrees.length ? entrees.map((a) => `
        <div class="champ-lecture">
          <div class="entre-deux">
            <strong>${esc(LIBELLES_ACTION[a.action] || a.action)}</strong>
            <span class="discret">${dateHeure(a.created_at)}</span>
          </div>
          <div class="discret">${esc(a.user_nom || 'Anonyme')}${a.details ? ` — ${esc(a.details)}` : ''}</div>
        </div>`).join('')
        : '<p class="discret">Journal vide.</p>'}
    </div>`);
}

// --- File d'attente ---------------------------------------------------------

const LIBELLES_OPERATION = {
  prise: 'Prise en compte', restitution: 'Restitution', incident: 'Signalement',
};

async function vueFileAttente() {
  const file = fileEnAttente();

  poser(`
    ${retour('/', 'Retour au tableau de bord')}
    <header class="entete">
      <h1>Opérations en attente</h1>
      <p class="sous-titre">Saisies enregistrées sur cet appareil, pas encore transmises au serveur</p>
    </header>
    <div id="zone-message"></div>
    ${file.length ? `
      <button class="btn" data-action="synchroniser" style="margin-bottom:16px">
        Transmettre maintenant</button>
      ${file.map((e) => `
        <article class="carte">
          <div class="entre-deux" style="margin-bottom:8px">
            <div class="pile">
              <strong>${esc(e.resume)}</strong>
              <span class="discret">${esc(LIBELLES_OPERATION[e.operation] || e.operation)}
                — saisi le ${dateHeure(e.date)}</span>
            </div>
          </div>
          ${e.erreur ? `
            ${message('erreur', e.erreur)}
            <p class="discret" style="margin-bottom:10px">Le serveur a refusé cette opération.
              Réessayer ne changera rien : abandonnez-la puis ressaisissez-la si besoin.</p>
            <button class="btn danger compact" data-action="abandonner" data-cle="${esc(e.cle)}">
              Abandonner cette saisie</button>` : `
            <p class="discret">En attente du réseau.</p>`}
        </article>`).join('')}`
      : vide("Aucune opération en attente. Tout a été transmis.", icones.attente)}`);

  surClic(async (action, data, e, cible) => {
    if (action === 'synchroniser') {
      await enAttente(cible, async () => {
        const bilan = await synchroniser();
        await vueFileAttente();
        if (bilan.transmises) {
          poserMessage('succes',
            `${bilan.transmises} opération${bilan.transmises > 1 ? 's' : ''} transmise${bilan.transmises > 1 ? 's' : ''}.`);
        } else if (!navigator.onLine) {
          poserMessage('attention', "Toujours hors ligne. Les saisies restent enregistrées.");
        } else if (bilan.echecs) {
          poserMessage('erreur',
            `${bilan.echecs} opération${bilan.echecs > 1 ? 's' : ''} refusée${bilan.echecs > 1 ? 's' : ''} par le serveur.`);
        }
      });
    } else if (action === 'abandonner') {
      if (!confirm("Abandonner définitivement cette saisie ? Elle ne sera pas enregistrée.")) return;
      abandonner(data.cle);
      await vueFileAttente();
      poserMessage('info', 'Saisie abandonnée.');
    }
  });
}


// --- Conservation des données ------------------------------------------------

// Le RGPD impose une durée de conservation définie et justifiée. Elle relève du
// DPO de la commune, pas du code : elle se règle donc ici. Comme une purge est
// irréversible, l'écran chiffre systématiquement ce qui serait supprimé avant
// que l'administrateur valide quoi que ce soit.

const DUREES_ACTIVITE = [
  [0, 'Illimitée — ne rien supprimer'],
  [12, '1 an'], [24, '2 ans'], [36, '3 ans'], [60, '5 ans'], [120, '10 ans'],
];
const DUREES_JOURNAL = [
  [0, 'Illimitée — ne rien supprimer'],
  [6, '6 mois'], [12, '1 an'], [24, '2 ans'], [36, '3 ans'], [60, '5 ans'],
];

async function vueConservation() {
  if (!session.peut('admin')) return vue404();
  chargement();
  const d = await api.conservation();
  const c = d.conservation;
  const active = c.activite_mois > 0 || c.journal_mois > 0;

  const options = (liste, valeur) => liste.map(([v, l]) =>
    `<option value="${v}"${v === valeur ? ' selected' : ''}>${l}</option>`).join('');

  poser(`
    ${retour('/reglages', 'Retour aux réglages')}
    <header class="entete">
      <h1>Conservation des données</h1>
      <p class="sous-titre">Durée au-delà de laquelle les données sont supprimées définitivement</p>
    </header>
    <div id="zone-message"></div>

    <div class="message info">
      L'application enregistre nominativement l'activité d'agents publics. Le RGPD
      impose une durée de conservation définie et justifiée : elle relève du délégué
      à la protection des données de votre commune.
    </div>

    <form id="form-conservation">
      <div class="carte">
        <div class="champ">
          <label for="activite">Historique d'activité</label>
          <select id="activite" name="activite">${options(DUREES_ACTIVITE, c.activite_mois)}</select>
          <p class="aide">Sorties terminées et incidents résolus. Les sorties en cours
            et les incidents ouverts ne sont jamais supprimés, quelle que soit leur ancienneté.</p>
        </div>
        <div class="champ" style="margin-bottom:0">
          <label for="journal">Journal d'activité</label>
          <select id="journal" name="journal">${options(DUREES_JOURNAL, c.journal_mois)}</select>
          <p class="aide">Connexions, créations de comptes, actions sensibles.</p>
        </div>
      </div>

      <div class="carte" id="apercu">${apercuPurge(d.a_purger, active)}</div>

      <button class="btn" type="submit">Enregistrer ces durées</button>
    </form>

    ${active ? `
      <div class="carte" style="margin-top:16px">
        <h2 style="margin-bottom:8px">Purge immédiate</h2>
        <p class="discret" style="margin-bottom:12px">
          La purge s'exécute automatiquement une fois par jour, après la sauvegarde.
          Ce bouton la déclenche sans attendre.</p>
        <button class="btn danger" data-action="purger">Purger maintenant</button>
      </div>` : ''}

    ${c.derniere_purge ? `
      <p class="discret" style="margin-top:14px;text-align:center">
        Dernière purge : ${dateHeure(c.derniere_purge)}</p>` : ''}`);

  const form = app.querySelector('#form-conservation');

  // Le chiffrage se met à jour à chaque changement, avant tout enregistrement.
  const rafraichirApercu = async () => {
    const choix = {
      activite_mois: Number(form.activite.value),
      journal_mois: Number(form.journal.value),
    };
    const bloc = app.querySelector('#apercu');
    bloc.innerHTML = '<p class="discret">Calcul en cours…</p>';
    try {
      const r = await api.simulerPurge(choix);
      bloc.innerHTML = apercuPurge(r.a_purger,
        choix.activite_mois > 0 || choix.journal_mois > 0);
    } catch (err) {
      bloc.innerHTML = message('erreur', err.message);
    }
  };
  form.activite.addEventListener('change', rafraichirApercu);
  form.journal.addEventListener('change', rafraichirApercu);

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    await enAttente(form.querySelector('button[type=submit]'), async () => {
      try {
        await api.definirConservation({
          activite_mois: Number(form.activite.value),
          journal_mois: Number(form.journal.value),
        });
        await vueConservation();
        poserMessage('succes',
          'Durées enregistrées. La purge s\'exécutera à la prochaine échéance quotidienne.');
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });

  surClic(async (action, data, e, cible) => {
    if (action !== 'purger') return;
    const r = await api.simulerPurge({});
    const total = r.a_purger.sorties + r.a_purger.incidents + r.a_purger.journal;
    if (total === 0) {
      poserMessage('info', "Aucune donnée n'a dépassé la durée de conservation.");
      return;
    }
    if (!confirm(`${nombre(total)} enregistrement(s) vont être supprimés définitivement.\n\n`
      + `Sorties : ${nombre(r.a_purger.sorties)}\n`
      + `Incidents résolus : ${nombre(r.a_purger.incidents)}\n`
      + `Entrées de journal : ${nombre(r.a_purger.journal)}\n\n`
      + `Cette action est irréversible. Confirmer ?`)) return;

    await enAttente(cible, async () => {
      try {
        const bilan = await api.purger();
        await vueConservation();
        poserMessage('succes',
          `${nombre(bilan.supprime.sorties + bilan.supprime.incidents + bilan.supprime.journal)} enregistrement(s) supprimé(s).`);
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });
}

function apercuPurge(b, active) {
  if (!active) {
    return `<h2 style="margin-bottom:8px">Effet</h2>
      <p class="discret">Aucune durée définie : rien ne sera supprimé.</p>`;
  }
  const total = b.sorties + b.incidents + b.journal;
  if (total === 0) {
    return `<h2 style="margin-bottom:8px">Effet</h2>
      <p class="discret">Aucune donnée existante ne dépasse ces durées.</p>`;
  }
  return `
    <h2 style="margin-bottom:10px">Ce qui serait supprimé aujourd'hui</h2>
    ${message('attention', `${nombre(total)} enregistrement(s), définitivement.`)}
    <div class="champ-lecture"><div class="cle">Sorties terminées</div>
      <div class="val">${nombre(b.sorties)}</div></div>
    <div class="champ-lecture"><div class="cle">Incidents résolus</div>
      <div class="val">${nombre(b.incidents)}</div></div>
    <div class="champ-lecture"><div class="cle">Entrées de journal</div>
      <div class="val">${nombre(b.journal)}</div></div>`;
}

// --- Réglages --------------------------------------------------------------

async function vueReglages() {
  const u = session.utilisateur();

  poser(`
    <header class="entete">
      <h1>Réglages</h1>
      <p class="sous-titre">${esc(u.prenom)} ${esc(u.nom)} — ${esc(LIBELLES_ROLE[u.role] || u.role)}</p>
    </header>
    <div id="zone-message"></div>

    ${u.must_change_password
      ? message('attention', 'Votre mot de passe est provisoire. Changez-le dès maintenant.')
      : ''}

    <div class="carte">
      <h2 style="margin-bottom:12px">Changer mon mot de passe</h2>
      <form id="form-mdp">
        <div class="champ">
          <label for="actuel">Mot de passe actuel</label>
          <input type="password" id="actuel" name="actuel" autocomplete="current-password" required>
        </div>
        <div class="champ">
          <label for="nouveau">Nouveau mot de passe</label>
          <input type="password" id="nouveau" name="nouveau" autocomplete="new-password" required>
          <p class="aide">Au moins 10 caractères, dont une lettre et un chiffre.</p>
        </div>
        <div class="champ">
          <label for="confirmation">Confirmation</label>
          <input type="password" id="confirmation" name="confirmation" autocomplete="new-password" required>
        </div>
        <button class="btn" type="submit">Changer le mot de passe</button>
      </form>
    </div>

    ${session.peut('chef') ? `
      <div class="carte">
        <h2 style="margin-bottom:6px">Exporter les données</h2>
        <p class="discret" style="margin-bottom:14px">Fichiers CSV, lisibles directement
          dans Excel ou LibreOffice.</p>
        <div class="duo" style="margin-bottom:12px">
          <button class="btn secondaire compact" style="width:100%"
                  data-action="export" data-fichier="prises">Historique des sorties</button>
          <button class="btn secondaire compact" style="width:100%"
                  data-action="export" data-fichier="parc">État du parc</button>
        </div>
        <div class="duo" style="margin-bottom:0">
          <button class="btn secondaire compact" style="width:100%"
                  data-action="export" data-fichier="incidents">Incidents</button>
          <button class="btn secondaire compact" style="width:100%"
                  data-action="export" data-fichier="entretiens">Entretiens et coûts</button>
        </div>
      </div>
      <a class="btn secondaire" href="/agents" style="margin-bottom:12px">
        ${icones.agent} Gérer les agents</a>` : ''}
    ${session.peut('admin') ? `
      <a class="btn secondaire" href="/journal" style="margin-bottom:12px">
        ${icones.historique} Journal d'activité</a>
      <a class="btn secondaire" href="/conservation" style="margin-bottom:12px">
        ${icones.bouclier} Conservation des données</a>` : ''}

    <button class="btn danger" data-action="deconnexion">${icones.sortie} Se déconnecter</button>`);

  const form = app.querySelector('#form-mdp');
  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    if (form.nouveau.value !== form.confirmation.value) {
      poserMessage('erreur', 'La confirmation ne correspond pas au nouveau mot de passe.');
      return;
    }
    await enAttente(form.querySelector('button[type=submit]'), async () => {
      try {
        await api.changerMotDePasse(form.actuel.value, form.nouveau.value);
        // Le serveur ferme toutes les sessions, y compris celle-ci.
        session.fermer();
        await aller('/connexion', true);
        poserMessage('succes',
          'Mot de passe modifié. Reconnectez-vous avec le nouveau.');
      } catch (err) {
        poserMessage('erreur', err.message);
      }
    });
  });

  surClic(async (action, data) => {
    if (action === 'export') {
      await telecharger(`/api/v1/export/${data.fichier}.csv`,
        `vlpm-${data.fichier}.csv`);
      return;
    }
    if (action !== 'deconnexion') return;
    try {
      await api.deconnexion();
    } catch {
      // Même si le serveur est injoignable, on ferme la session locale.
    }
    session.fermer();
    aller('/connexion', true);
  });
}

// --- Démarrage -------------------------------------------------------------

// Le service worker met l'interface en cache : sans lui, l'application ne se
// chargerait tout simplement pas hors réseau. Son absence n'est pas bloquante
// (navigation privée, contexte non sécurisé) : seul le hors-ligne est perdu.
if ('serviceWorker' in navigator) {
  navigator.serviceWorker.register('/sw.js').catch((err) => {
    console.warn('Mode hors ligne indisponible :', err.message);
  });
}

// Le bandeau d'état doit suivre les changements de connectivité et de file.
// Un simple re-rendu de l'écran courant suffit à le remettre à jour.
window.addEventListener('online', () => rendre());
window.addEventListener('offline', () => rendre());
surChangement(() => {
  const bandeau = app.querySelector('.bandeau');
  const page = app.querySelector('.page');
  if (!page) return;
  const html = bandeauReseau();
  if (bandeau) bandeau.outerHTML = html;
  else if (html) page.insertAdjacentHTML('afterbegin', html);
});

if (session.connecte()) {
  demarrerSynchronisation(async (bilan) => {
    // Le rendu vient d'abord : il reconstruit la page, et effacerait un
    // message posé avant lui.
    await rendre();
    if (bilan.transmises) {
      const n = bilan.transmises;
      poserMessage('succes', n > 1
        ? `${n} opérations enregistrées hors ligne ont été transmises.`
        : `L'opération enregistrée hors ligne a été transmise.`);
    }
    if (bilan.echecs) {
      const n = bilan.echecs;
      poserMessage('erreur', n > 1
        ? `${n} opérations ont été refusées par le serveur. Consultez la file d'attente.`
        : `Une opération a été refusée par le serveur. Consultez la file d'attente.`);
    }
  });
}

// Le profil est rafraîchi au chargement : un rôle modifié ou un compte
// désactivé côté serveur doit se refléter sans attendre la reconnexion. Hors
// ligne, l'échec est sans conséquence : le profil mémorisé fait foi.
if (session.connecte()) {
  api.moi()
    .then((rep) => session.majUtilisateur(rep.user))
    .catch(() => {})
    .finally(rendre);
} else {
  rendre();
}
