package fr.assopmf.vlpm

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Réglages locaux de l'application.
 *
 * Le stockage est chiffré : il conserve l'adresse du serveur du service, qui
 * renseigne sur l'organisation, et servira au jeton de session si l'interface
 * web délègue un jour sa conservation à l'application. Un terminal de service
 * peut être perdu ou saisi, et un fichier de préférences en clair se lit sans
 * déverrouiller l'appareil dès lors qu'on a un accès physique.
 */
class Reglages(contexte: Context) {

    private val prefs: SharedPreferences = try {
        val cle = MasterKey.Builder(contexte)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            contexte,
            "vlpm_reglages",
            cle,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    } catch (e: Exception) {
        // Le chiffrement peut échouer sur un terminal dont le magasin de clés
        // est corrompu — cas rare mais constaté. Mieux vaut une application
        // qui fonctionne en clair qu'une application qui refuse de démarrer :
        // la donnée en jeu est une adresse de serveur, pas un secret.
        contexte.getSharedPreferences("vlpm_reglages_clair", Context.MODE_PRIVATE)
    }

    var adresseServeur: String?
        get() = prefs.getString(CLE_SERVEUR, null)
        set(valeur) = prefs.edit().putString(CLE_SERVEUR, valeur).apply()

    fun oublierServeur() = prefs.edit().remove(CLE_SERVEUR).apply()

    val estConfigure: Boolean
        get() = !adresseServeur.isNullOrBlank()

    private companion object {
        const val CLE_SERVEUR = "adresse_serveur"
    }
}
