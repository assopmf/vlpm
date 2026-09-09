package fr.assopmf.vlpm

import java.net.HttpURLConnection
import java.net.URL

/**
 * Normalisation et vérification de l'adresse du serveur.
 *
 * Les agents saisiront « vlpm.villeexemple.fr », « VLPM.villeexemple.fr/ » ou
 * l'adresse complète avec le protocole. Toutes doivent aboutir, sans quoi le
 * premier écran devient un obstacle plutôt qu'une formalité.
 */
object Adresse {

    /** Ajoute le protocole si absent et retire la barre oblique finale. */
    fun normaliser(saisie: String): String {
        var a = saisie.trim()
        if (a.isEmpty()) return a
        if (!a.startsWith("http://", true) && !a.startsWith("https://", true)) {
            // HTTPS par défaut : une commune qui n'a que du HTTP le saisira
            // explicitement, plutôt que l'inverse.
            a = "https://$a"
        }
        return a.trimEnd('/')
    }

    fun estChiffree(adresse: String) = adresse.startsWith("https://", true)

    sealed class Resultat {
        object Valide : Resultat()
        object Injoignable : Resultat()
        object PasUnServeurVLPM : Resultat()
    }

    /**
     * Interroge /healthz, seule route ouverte sans authentification.
     *
     * Vérifier que l'adresse répond ne suffit pas : un portail captif ou un
     * site quelconque répondrait aussi. On contrôle donc que la réponse est
     * bien celle d'un serveur VLPM, faute de quoi l'agent découvrirait son
     * erreur seulement à l'écran de connexion.
     */
    fun verifier(adresse: String): Resultat {
        return try {
            val connexion = (URL("$adresse/healthz").openConnection() as HttpURLConnection).apply {
                connectTimeout = 8_000
                readTimeout = 8_000
                requestMethod = "GET"
                setRequestProperty("Accept", "application/json")
                instanceFollowRedirects = true
            }
            try {
                val code = connexion.responseCode
                val corps = if (code in 200..299) {
                    connexion.inputStream.bufferedReader().use { it.readText() }
                } else {
                    connexion.errorStream?.bufferedReader()?.use { it.readText() } ?: ""
                }
                // 503 signale une base injoignable : le serveur est bien un
                // VLPM, il a seulement un problème. L'adresse est donc juste.
                when {
                    corps.contains("\"statut\"") -> Resultat.Valide
                    code == 503 && corps.contains("base") -> Resultat.Valide
                    else -> Resultat.PasUnServeurVLPM
                }
            } finally {
                connexion.disconnect()
            }
        } catch (e: Exception) {
            Resultat.Injoignable
        }
    }
}
