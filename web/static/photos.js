// Capture et préparation des photos de constat.
//
// L'image est redimensionnée et réencodée dans le navigateur avant d'être
// transmise. Trois raisons, dans cet ordre d'importance :
//
// 1. Le réencodage supprime les métadonnées EXIF, dont les coordonnées GPS.
//    Une photo prise avec un téléphone de service porte la position exacte de
//    l'intervention ; elle n'a rien à faire dans un constat de carrosserie.
// 2. Une photo de téléphone pèse 4 à 12 Mo ; réduite à 1600 px, elle tombe
//    sous 500 Ko, ce qui compte sur un réseau mobile en bord de route.
// 3. Le format de sortie est du JPEG, que le serveur sait reconnaître, là où
//    un iPhone produit du HEIC par défaut.

import { session } from './api.js';

const COTE_MAX = 1600;
const QUALITE = 0.82;

// preparerPhoto réduit une image et renvoie un Blob JPEG.
export async function preparerPhoto(fichier) {
  const image = await chargerImage(fichier);
  const { width, height } = dimensionsReduites(image.width, image.height);

  const toile = document.createElement('canvas');
  toile.width = width;
  toile.height = height;
  const ctx = toile.getContext('2d');
  ctx.drawImage(image, 0, 0, width, height);
  if (image.close) image.close();

  const blob = await new Promise((resolve) =>
    toile.toBlob(resolve, 'image/jpeg', QUALITE));
  if (!blob) throw new Error("Cette image n'a pas pu être préparée.");
  return blob;
}

async function chargerImage(fichier) {
  // createImageBitmap gère l'orientation EXIF, ce que <img> ne fait pas
  // toujours : sans cela, une photo prise en portrait ressort couchée.
  if (window.createImageBitmap) {
    try {
      return await createImageBitmap(fichier, { imageOrientation: 'from-image' });
    } catch {
      // Format non géré par le navigateur : on tente la voie classique.
    }
  }
  const url = URL.createObjectURL(fichier);
  try {
    return await new Promise((resolve, reject) => {
      const img = new Image();
      img.onload = () => resolve(img);
      img.onerror = () => reject(new Error(
        "Ce fichier n'est pas une image que votre navigateur sait lire."));
      img.src = url;
    });
  } finally {
    URL.revokeObjectURL(url);
  }
}

function dimensionsReduites(largeur, hauteur) {
  const plusGrand = Math.max(largeur, hauteur);
  if (plusGrand <= COTE_MAX) return { width: largeur, height: hauteur };
  const facteur = COTE_MAX / plusGrand;
  return {
    width: Math.round(largeur * facteur),
    height: Math.round(hauteur * facteur),
  };
}

// envoyerPhoto transmet une photo déjà préparée pour un incident donné.
export async function envoyerPhoto(incidentID, blob) {
  const formulaire = new FormData();
  formulaire.append('photo', blob, 'constat.jpg');

  const reponse = await fetch(`/api/v1/incidents/${incidentID}/photos`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${session.jeton()}` },
    body: formulaire,
  });
  const texte = await reponse.text();
  let donnees = null;
  try {
    donnees = texte ? JSON.parse(texte) : null;
  } catch {
    throw new Error('Réponse inattendue du serveur.');
  }
  if (!reponse.ok) {
    throw new Error(donnees?.erreur || `Envoi refusé (${reponse.status}).`);
  }
  return donnees;
}

// urlPhoto récupère l'image en passant l'en-tête d'authentification, puis la
// rend affichable : une balise <img src> nue serait refusée par l'API.
export async function urlPhoto(id) {
  const reponse = await fetch(`/api/v1/photos/${id}`, {
    headers: { Authorization: `Bearer ${session.jeton()}` },
  });
  if (!reponse.ok) throw new Error('Photo indisponible.');
  return URL.createObjectURL(await reponse.blob());
}
