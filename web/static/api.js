// Client HTTP de l'API VLPM.
//
// Le jeton de session vit dans localStorage et part dans l'en-tête
// Authorization. C'est le même mécanisme que celui de la future application
// Android, et l'absence de cookie d'authentification écarte tout risque CSRF.

const CLE_JETON = 'vlpm.jeton';
const CLE_USER = 'vlpm.user';

export const session = {
  jeton: () => localStorage.getItem(CLE_JETON),

  utilisateur() {
    try {
      return JSON.parse(localStorage.getItem(CLE_USER) || 'null');
    } catch {
      return null; // stockage corrompu : on repart d'une session vierge
    }
  },

  ouvrir(jeton, user) {
    localStorage.setItem(CLE_JETON, jeton);
    localStorage.setItem(CLE_USER, JSON.stringify(user));
  },

  majUtilisateur(user) {
    localStorage.setItem(CLE_USER, JSON.stringify(user));
  },

  fermer() {
    localStorage.removeItem(CLE_JETON);
    localStorage.removeItem(CLE_USER);
  },

  connecte: () => Boolean(localStorage.getItem(CLE_JETON)),

  // peut : hiérarchie agent < chef < admin, identique à celle du serveur.
  peut(role) {
    const rang = { agent: 1, chef: 2, admin: 3 };
    const u = session.utilisateur();
    return Boolean(u) && (rang[u.role] || 0) >= rang[role];
  },
};

// ErreurAPI porte le message rédigé par le serveur, déjà en français et
// destiné à être affiché tel quel à l'agent.
export class ErreurAPI extends Error {
  constructor(message, statut, code) {
    super(message);
    this.name = 'ErreurAPI';
    this.statut = statut;
    this.code = code;
  }
}

// surSessionExpiree est branché par l'application pour renvoyer à l'écran de
// connexion sans que chaque appel ait à s'en préoccuper.
let surSessionExpiree = () => {};
export function brancherExpiration(fn) {
  surSessionExpiree = fn;
}

async function requete(methode, chemin, corps) {
  const options = { method: methode, headers: {} };
  const jeton = session.jeton();
  if (jeton) options.headers.Authorization = `Bearer ${jeton}`;
  if (corps !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(corps);
  }

  let reponse;
  try {
    reponse = await fetch(`/api/v1${chemin}`, options);
  } catch {
    throw new ErreurAPI(
      "Serveur injoignable. Vérifiez votre connexion réseau.", 0, 'reseau');
  }

  if (reponse.status === 401 && session.connecte()) {
    session.fermer();
    surSessionExpiree();
    throw new ErreurAPI('Votre session a expiré. Reconnectez-vous.', 401, 'session_expiree');
  }
  if (reponse.status === 204) return null;

  const texte = await reponse.text();
  let donnees = null;
  if (texte) {
    try {
      donnees = JSON.parse(texte);
    } catch {
      throw new ErreurAPI('Réponse inattendue du serveur.', reponse.status, 'reponse_invalide');
    }
  }
  if (!reponse.ok) {
    const msg = donnees?.erreur || `Erreur ${reponse.status}.`;
    throw new ErreurAPI(msg, reponse.status, donnees?.code);
  }
  return donnees;
}

export const api = {
  connexion: (matricule, motDePasse) =>
    requete('POST', '/auth/login', { matricule, mot_de_passe: motDePasse }),
  deconnexion: () => requete('POST', '/auth/logout'),
  moi: () => requete('GET', '/moi'),
  sessions: () => requete('GET', '/moi/sessions'),
  revoquerSession: (id) => requete('DELETE', `/moi/sessions/${id}`),
  revoquerAutresSessions: () => requete('POST', '/moi/sessions/revoquer-autres'),

  changerMotDePasse: (actuel, nouveau) =>
    requete('POST', '/moi/mot-de-passe', {
      mot_de_passe_actuel: actuel, nouveau_mot_de_passe: nouveau,
    }),

  stats: () => requete('GET', '/stats'),
  alertes: () => requete('GET', '/alertes'),
  vehicules: (statut) =>
    requete('GET', `/vehicules${statut && statut !== 'tous' ? `?statut=${statut}` : ''}`),
  vehicule: (id) => requete('GET', `/vehicules/${id}`),
  creerVehicule: (v) => requete('POST', '/vehicules', v),
  modifierVehicule: (id, v) => requete('PATCH', `/vehicules/${id}`, v),
  scanner: (jeton) => requete('GET', `/scan/${encodeURIComponent(jeton)}`),

  prises: (params = {}) => {
    const q = new URLSearchParams(
      Object.entries(params).filter(([, v]) => v !== '' && v != null));
    return requete('GET', `/prises${q.toString() ? `?${q}` : ''}`);
  },
  prendreEnCompte: (donnees) => requete('POST', '/prises', donnees),
  restituer: (id, donnees) => requete('POST', `/prises/${id}/restitution`, donnees),

  incidents: (params = {}) => {
    const q = new URLSearchParams(
      Object.entries(params).filter(([, v]) => v !== '' && v != null));
    return requete('GET', `/incidents${q.toString() ? `?${q}` : ''}`);
  },
  signalerIncident: (i) => requete('POST', '/incidents', i),
  resoudreIncident: (id) => requete('POST', `/incidents/${id}/resolution`),

  entretiens: (vehicule) =>
    requete('GET', `/entretiens${vehicule ? `?vehicule=${vehicule}` : ''}`),
  creerEntretien: (m) => requete('POST', '/entretiens', m),

  agents: (inactifs) => requete('GET', `/agents${inactifs ? '?inactifs=1' : ''}`),
  creerAgent: (a) => requete('POST', '/agents', a),
  modifierAgent: (id, a) => requete('PATCH', `/agents/${id}`, a),
  reinitialiserMotDePasse: (id) => requete('POST', `/agents/${id}/mot-de-passe`),

  journal: () => requete('GET', '/journal'),

  conservation: () => requete('GET', '/conservation'),
  definirConservation: (c) => requete('PATCH', '/conservation', c),
  simulerPurge: (c) => requete('POST', '/conservation/simulation', c),
  purger: () => requete('POST', '/conservation/purger'),

  notifications: () => requete('GET', '/notifications'),
  definirNotifications: (n) => requete('PATCH', '/notifications', n),
  envoyerReleve: () => requete('POST', '/notifications/envoyer'),
};
