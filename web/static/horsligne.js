// File d'attente des opérations réalisées sans réseau.
//
// Une prise en compte dans un parking souterrain doit aboutir. On l'enregistre
// localement, on la rejoue dès que le réseau revient, et le serveur garantit
// qu'un rejeu ne crée pas de doublon grâce à la clé jointe à chaque opération.
//
// Le stockage passe par localStorage plutôt qu'IndexedDB : la file compte au
// plus quelques opérations, et l'API synchrone reste lisible pour qui reprend
// le projet.

import { api, ErreurAPI } from './api.js';

const CLE_FILE = 'vlpm.file';
const CLE_PARC = 'vlpm.parc';

// --- Identifiants d'opération ----------------------------------------------

function nouvelleCle() {
  if (crypto.randomUUID) return crypto.randomUUID();
  // Repli pour les contextes non sécurisés, où randomUUID est absent.
  const o = crypto.getRandomValues(new Uint8Array(16));
  return [...o].map((b) => b.toString(16).padStart(2, '0')).join('');
}

// --- File ------------------------------------------------------------------

function lireFile() {
  try {
    const brut = JSON.parse(localStorage.getItem(CLE_FILE) || '[]');
    return Array.isArray(brut) ? brut : [];
  } catch {
    return []; // stockage corrompu : on repart d'une file vide
  }
}

function ecrireFile(file) {
  localStorage.setItem(CLE_FILE, JSON.stringify(file));
  previenir();
}

export function fileEnAttente() {
  return lireFile();
}

export function nombreEnAttente() {
  return lireFile().length;
}

// empiler ajoute une opération à rejouer plus tard.
// `operation` vaut 'prise', 'restitution' ou 'incident'.
export function empiler(operation, donnees, resume) {
  const file = lireFile();
  file.push({
    cle: nouvelleCle(),
    operation,
    donnees,
    resume,            // texte affiché à l'agent, ex. « TV1 pris en compte »
    date: new Date().toISOString(),
    tentatives: 0,
    erreur: '',
  });
  ecrireFile(file);
  return file[file.length - 1];
}

function retirer(cle) {
  ecrireFile(lireFile().filter((e) => e.cle !== cle));
}

export function abandonner(cle) {
  retirer(cle);
}

// --- Synchronisation --------------------------------------------------------

let synchronisationEnCours = false;

// synchroniser rejoue la file dans l'ordre. Renvoie le bilan de l'opération.
export async function synchroniser() {
  if (synchronisationEnCours || !navigator.onLine) {
    return { transmises: 0, echecs: 0, restantes: nombreEnAttente() };
  }
  synchronisationEnCours = true;
  let transmises = 0;
  let echecs = 0;

  try {
    // On travaille sur une copie : la file peut être modifiée entre-temps.
    for (const entree of lireFile()) {
      try {
        await transmettre(entree);
        retirer(entree.cle);
        transmises += 1;
      } catch (err) {
        if (err instanceof ErreurAPI && err.statut === 0) {
          // Réseau de nouveau absent : on s'arrête, le reste attendra.
          break;
        }
        // Le serveur a répondu et refuse l'opération : véhicule déjà pris par
        // un collègue, kilométrage aberrant, session expirée. Réessayer n'y
        // changera rien, l'agent doit trancher. On garde l'entrée avec son
        // motif de refus.
        marquerEchec(entree.cle, err.message);
        echecs += 1;
      }
    }
  } finally {
    synchronisationEnCours = false;
  }
  return { transmises, echecs, restantes: nombreEnAttente() };
}

async function transmettre(entree) {
  switch (entree.operation) {
    case 'prise':
      return api.prendreEnCompte(entree.donnees);
    case 'restitution':
      return api.restituer(entree.donnees.checkout_id, entree.donnees.corps);
    case 'incident':
      return api.signalerIncident(entree.donnees);
    default:
      throw new ErreurAPI(`Opération inconnue : ${entree.operation}`, 400, 'operation_inconnue');
  }
}

function marquerEchec(cle, message) {
  const file = lireFile();
  const entree = file.find((e) => e.cle === cle);
  if (!entree) return;
  entree.tentatives += 1;
  entree.erreur = message;
  ecrireFile(file);
}

// --- Instantané du parc ------------------------------------------------------

// Le dernier état connu du parc, pour afficher quelque chose hors ligne.
export function memoriserParc(vehicules, stats) {
  try {
    localStorage.setItem(CLE_PARC, JSON.stringify({
      vehicules, stats, date: new Date().toISOString(),
    }));
  } catch {
    // Quota dépassé ou navigation privée : on se passe de l'instantané.
  }
}

export function parcMemorise() {
  try {
    return JSON.parse(localStorage.getItem(CLE_PARC) || 'null');
  } catch {
    return null;
  }
}

// --- Notification de changement ---------------------------------------------

const abonnes = new Set();

export function surChangement(fn) {
  abonnes.add(fn);
  return () => abonnes.delete(fn);
}

function previenir() {
  for (const fn of abonnes) {
    try {
      fn(nombreEnAttente());
    } catch {
      // Un abonné défaillant ne doit pas empêcher les autres d'être avertis.
    }
  }
}

// --- Démarrage ---------------------------------------------------------------

// Le retour du réseau déclenche le rejeu. L'événement 'online' est optimiste
// (il signale une interface réseau, pas une connexion qui aboutit) : la
// synchronisation échoue proprement et retentera si ce n'est qu'une apparence.
export function demarrerSynchronisation(surBilan) {
  const lancer = async () => {
    if (!nombreEnAttente()) return;
    const bilan = await synchroniser();
    if (bilan.transmises || bilan.echecs) surBilan?.(bilan);
  };

  window.addEventListener('online', lancer);
  // Un retour sur l'application après une mise en veille prolongée.
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) lancer();
  });
  lancer();
}

export { nouvelleCle };
