// Briques d'affichage réutilisées par les vues.

// esc protège toute donnée saisie par un agent avant insertion dans du HTML.
// Tout ce qui vient du serveur passe par ici : nom d'agent, observations,
// description d'incident, immatriculation.
export function esc(v) {
  if (v === null || v === undefined) return '';
  return String(v)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

export const icones = {
  vehicule: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 17h-2v-6l2-5h9l4 5h3a2 2 0 0 1 2 2v4h-2"/><circle cx="7" cy="17" r="2"/><circle cx="17" cy="17" r="2"/><path d="M9 17h6"/></svg>`,
  compteur: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 12l4-3"/></svg>`,
  calendrier: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="16" rx="2"/><path d="M16 3v4M8 3v4M3 11h18"/></svg>`,
  agent: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="8" r="4"/><path d="M4 21v-1a6 6 0 0 1 6-6h4a6 6 0 0 1 6 6v1"/></svg>`,
  qr: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3h-3zM18 18h3v3h-3z"/></svg>`,
  historique: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/></svg>`,
  accueil: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 10l9-7 9 7v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="M9 21v-8h6v8"/></svg>`,
  alerte: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/></svg>`,
  reglages: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-2.9 1.2v.2a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0-1.2-2.9H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 2.9 1.2l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/></svg>`,
  fleche: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5M12 19l-7-7 7-7"/></svg>`,
  sortie: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="M16 17l5-5-5-5M21 12H9"/></svg>`,
  bouclier: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>`,
};

export const LIBELLES_STATUT = {
  disponible: 'Disponible',
  en_service: 'En service',
  maintenance: 'Maintenance',
  hors_service: 'Hors service',
};

export const LIBELLES_INCIDENT = {
  dommage: 'Dommage', panne: 'Panne', proprete: 'Propreté',
  carburant: 'Carburant', equipement: 'Équipement', autre: 'Autre',
};

export const LIBELLES_GRAVITE = {
  mineur: 'Mineur', majeur: 'Majeur', immobilisant: 'Immobilisant',
};

export const LIBELLES_ENTRETIEN = {
  revision: 'Révision', reparation: 'Réparation',
  controle_technique: 'Contrôle technique', pneus: 'Pneumatiques',
  carburant: 'Carburant', autre: 'Autre',
};

// nombre insère une espace fine entre les milliers : « 29 800 km ».
export function nombre(n) {
  if (n === null || n === undefined || n === '') return '—';
  return Number(n).toLocaleString('fr-FR');
}

export function dateHeure(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString('fr-FR', {
    day: '2-digit', month: '2-digit', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  });
}

export function dateCourte(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleDateString('fr-FR');
}

export function euros(centimes) {
  return ((centimes || 0) / 100).toLocaleString('fr-FR', {
    style: 'currency', currency: 'EUR',
  });
}

export function badge(statut, libelles = LIBELLES_STATUT) {
  return `<span class="badge ${esc(statut)}">${esc(libelles[statut] || statut)}</span>`;
}

export function message(type, texte) {
  return `<div class="message ${type}" role="${type === 'erreur' ? 'alert' : 'status'}">${esc(texte)}</div>`;
}

export function vide(texte, icone = icones.vehicule) {
  return `<div class="vide">${icone}<p>${esc(texte)}</p></div>`;
}

// carteVehicule : la brique de la liste du parc.
// Le bouton d'action n'apparaît que si l'agent peut réellement agir.
export function carteVehicule(v, { action = true } = {}) {
  const enCours = v.checkout_en_cours;
  const lignes = [];

  lignes.push(`
    <div class="ligne">
      <div class="icone-vehicule">${icones.vehicule}</div>
      <div class="infos">
        <div class="entre-deux">
          <div>
            <div class="code">${esc(v.code)}</div>
            <div class="modele">${esc([v.marque, v.modele].filter(Boolean).join(' ')) || '&nbsp;'}</div>
          </div>
          ${badge(v.statut)}
        </div>
      </div>
    </div>`);

  const meta = [
    `<span>${icones.compteur} ${nombre(v.km)} km</span>`,
  ];
  if (v.immatriculation) meta.push(`<span>${esc(v.immatriculation)}</span>`);
  if (v.prochain_ct) {
    meta.push(`<span>${icones.calendrier} CT ${dateCourte(v.prochain_ct)}</span>`);
  }
  lignes.push(`<div class="meta">${meta.join('')}</div>`);

  if (enCours) {
    lignes.push(`<div class="meta"><span>${icones.agent} ${esc(enCours.user_nom)} — depuis ${dateHeure(enCours.started_at)}</span></div>`);
  }
  if (v.incidents_ouverts > 0) {
    const n = v.incidents_ouverts;
    lignes.push(`<div class="alerte-incident">${icones.alerte}
      ${n} incident${n > 1 ? 's' : ''} non résolu${n > 1 ? 's' : ''}</div>`);
  }

  if (action) {
    if (v.statut === 'disponible') {
      lignes.push(`<button class="btn" data-action="prendre" data-id="${v.id}">
        ${icones.qr} Prendre en compte</button>`);
    } else if (enCours) {
      lignes.push(`<button class="btn secondaire" data-action="restituer" data-checkout="${enCours.id}">
        ${icones.sortie} Restituer</button>`);
    }
  }

  return `<article class="carte vehicule" data-action="fiche" data-id="${v.id}">${lignes.join('')}</article>`;
}
