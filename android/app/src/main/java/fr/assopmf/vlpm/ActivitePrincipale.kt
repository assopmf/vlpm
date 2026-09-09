package fr.assopmf.vlpm

import android.Manifest
import android.annotation.SuppressLint
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.MediaStore
import android.view.View
import android.webkit.*
import android.widget.*
import androidx.activity.OnBackPressedCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import java.io.File
import java.util.concurrent.Executors

class ActivitePrincipale : AppCompatActivity() {

    private lateinit var vueWeb: WebView
    private lateinit var ecranConfiguration: View
    private lateinit var reglages: Reglages

    private var retourFichier: ValueCallback<Array<Uri>>? = null
    private var photoEnCours: Uri? = null

    private val travailleur = Executors.newSingleThreadExecutor()

    // --- Scanner de QR code ---
    //
    // C'est la raison d'être de cette application : un navigateur refuse
    // l'accès à la caméra hors HTTPS, une application native non. Les
    // communes sans certificat disposent ainsi d'un scanner fonctionnel.
    private val scanner = registerForActivityResult(ScanContract()) { resultat ->
        val valeur = resultat.contents
        if (valeur == null) {
            renvoyerAuJS("null")
        } else {
            renvoyerAuJS(echapperPourJS(valeur))
        }
    }

    private val demandeCamera = registerForActivityResult(
        ActivityResultContracts.RequestPermission()
    ) { accordee ->
        if (accordee) lancerScanner()
        else {
            Toast.makeText(this, R.string.camera_refusee, Toast.LENGTH_LONG).show()
            renvoyerAuJS("null")
        }
    }

    private val choixFichier = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { resultat ->
        val uris = when {
            resultat.resultCode != RESULT_OK -> null
            resultat.data?.data != null -> arrayOf(resultat.data!!.data!!)
            // Aucune donnée renvoyée : l'appareil photo a écrit dans le
            // fichier que nous lui avions désigné.
            photoEnCours != null -> arrayOf(photoEnCours!!)
            else -> null
        }
        retourFichier?.onReceiveValue(uris)
        retourFichier = null
        photoEnCours = null
    }

    override fun onCreate(etat: Bundle?) {
        super.onCreate(etat)
        setContentView(R.layout.activite_principale)
        reglages = Reglages(this)

        vueWeb = findViewById(R.id.vue_web)
        ecranConfiguration = findViewById(R.id.ecran_configuration)
        preparerVueWeb()
        preparerConfiguration()

        val adresse = reglages.adresseServeur
        if (adresse != null) ouvrirServeur(adresse) else afficherConfiguration()

        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                // Le bouton retour doit parcourir l'historique de l'interface
                // web, sinon il ferme l'application dès le premier appui.
                if (vueWeb.visibility == View.VISIBLE && vueWeb.canGoBack()) vueWeb.goBack()
                else finish()
            }
        })
    }

    // --- Configuration ---

    private fun preparerConfiguration() {
        val champ = findViewById<EditText>(R.id.champ_adresse)
        val bouton = findViewById<Button>(R.id.bouton_valider)
        val erreur = findViewById<TextView>(R.id.message_erreur)
        champ.setText(reglages.adresseServeur ?: "")

        bouton.setOnClickListener {
            val saisie = Adresse.normaliser(champ.text.toString())
            if (saisie.isEmpty()) {
                afficherErreur(erreur, getString(R.string.erreur_adresse_vide))
                return@setOnClickListener
            }
            bouton.isEnabled = false
            bouton.text = getString(R.string.configuration_verification)
            erreur.visibility = View.GONE

            travailleur.execute {
                val resultat = Adresse.verifier(saisie)
                runOnUiThread {
                    bouton.isEnabled = true
                    bouton.text = getString(R.string.configuration_valider)
                    when (resultat) {
                        Adresse.Resultat.Valide -> {
                            reglages.adresseServeur = saisie
                            if (!Adresse.estChiffree(saisie)) {
                                Toast.makeText(this, R.string.avertissement_http,
                                    Toast.LENGTH_LONG).show()
                            }
                            ouvrirServeur(saisie)
                        }
                        Adresse.Resultat.Injoignable ->
                            afficherErreur(erreur, getString(R.string.erreur_injoignable))
                        Adresse.Resultat.PasUnServeurVLPM ->
                            afficherErreur(erreur, getString(R.string.erreur_pas_vlpm))
                    }
                }
            }
        }
    }

    private fun afficherErreur(vue: TextView, texte: String) {
        vue.text = texte
        vue.visibility = View.VISIBLE
    }

    private fun afficherConfiguration() {
        ecranConfiguration.visibility = View.VISIBLE
        vueWeb.visibility = View.GONE
    }

    private fun ouvrirServeur(adresse: String) {
        ecranConfiguration.visibility = View.GONE
        vueWeb.visibility = View.VISIBLE
        vueWeb.loadUrl(adresse)
    }

    // --- Vue web ---

    @SuppressLint("SetJavaScriptEnabled")
    private fun preparerVueWeb() {
        vueWeb.settings.apply {
            javaScriptEnabled = true
            // Indispensable : la file d'attente hors ligne et l'instantané du
            // parc reposent sur localStorage.
            domStorageEnabled = true
            databaseEnabled = true
            loadWithOverviewMode = true
            useWideViewPort = true
            mediaPlaybackRequiresUserGesture = false
            // Repris par le serveur pour nommer l'appareil dans la liste des
            // sessions : l'agent y reconnaît son téléphone.
            userAgentString = "$userAgentString VLPM-Android/${BuildConfig.VERSION_NAME} " +
                "(Android ${Build.VERSION.RELEASE})"
        }
        vueWeb.addJavascriptInterface(PontAndroid(), "VLPMAndroid")

        vueWeb.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(
                vue: WebView, requete: WebResourceRequest,
            ): Boolean {
                val cible = requete.url.toString()
                val serveur = reglages.adresseServeur ?: return false
                // Un lien sortant s'ouvre dans le navigateur : l'application
                // n'a pas à devenir un navigateur généraliste.
                return if (!cible.startsWith(serveur)) {
                    try {
                        startActivity(Intent(Intent.ACTION_VIEW, requete.url))
                    } catch (e: Exception) {
                        Toast.makeText(this@ActivitePrincipale,
                            "Aucune application pour ouvrir ce lien", Toast.LENGTH_SHORT).show()
                    }
                    true
                } else false
            }

            override fun onReceivedError(
                vue: WebView, requete: WebResourceRequest, erreur: WebResourceError,
            ) {
                if (requete.isForMainFrame) {
                    Toast.makeText(this@ActivitePrincipale,
                        R.string.erreur_injoignable, Toast.LENGTH_LONG).show()
                }
            }
        }

        vueWeb.webChromeClient = object : WebChromeClient() {
            // Sans cela, le bouton « Photos » du constat ne fait rien.
            override fun onShowFileChooser(
                vue: WebView,
                retour: ValueCallback<Array<Uri>>,
                parametres: FileChooserParams,
            ): Boolean {
                retourFichier?.onReceiveValue(null)
                retourFichier = retour
                ouvrirSelecteurPhoto(parametres)
                return true
            }

            // getUserMedia n'est pas utilisé pour le scan — on passe par le
            // scanner natif — mais reste possible pour d'autres usages.
            override fun onPermissionRequest(requete: PermissionRequest) {
                runOnUiThread {
                    if (requete.resources.contains(PermissionRequest.RESOURCE_VIDEO_CAPTURE) &&
                        ContextCompat.checkSelfPermission(this@ActivitePrincipale,
                            Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED
                    ) requete.grant(requete.resources) else requete.deny()
                }
            }
        }
    }

    private fun ouvrirSelecteurPhoto(parametres: WebChromeClient.FileChooserParams) {
        val galerie = parametres.createIntent().apply {
            type = "image/*"
            if (parametres.mode == WebChromeClient.FileChooserParams.MODE_OPEN_MULTIPLE) {
                putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
            }
        }
        val intents = mutableListOf<Intent>()

        // L'appareil photo n'est proposé que si la permission est déjà
        // accordée : la demander depuis un sélecteur de fichiers dérouterait.
        if (ContextCompat.checkSelfPermission(this, Manifest.permission.CAMERA)
            == PackageManager.PERMISSION_GRANTED
        ) {
            try {
                val dossier = File(cacheDir, "photos").apply { mkdirs() }
                val fichier = File.createTempFile("constat_", ".jpg", dossier)
                photoEnCours = FileProvider.getUriForFile(
                    this, "$packageName.fichiers", fichier)
                intents += Intent(MediaStore.ACTION_IMAGE_CAPTURE)
                    .putExtra(MediaStore.EXTRA_OUTPUT, photoEnCours)
            } catch (e: Exception) {
                photoEnCours = null
            }
        }

        val choix = Intent.createChooser(galerie, "Photo du constat").apply {
            if (intents.isNotEmpty()) {
                putExtra(Intent.EXTRA_INITIAL_INTENTS, intents.toTypedArray())
            }
        }
        choixFichier.launch(choix)
    }

    // --- Pont JavaScript ---

    inner class PontAndroid {
        /**
         * Appelé par l'interface web pour lancer le scanner natif.
         * Le résultat revient par window.__vlpmScanResultat.
         */
        @JavascriptInterface
        fun scannerQR() {
            runOnUiThread {
                if (ContextCompat.checkSelfPermission(this@ActivitePrincipale,
                        Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED
                ) lancerScanner() else demandeCamera.launch(Manifest.permission.CAMERA)
            }
        }

        /** Permet à l'interface d'adapter son affichage. */
        @JavascriptInterface
        fun estApplicationNative(): Boolean = true

        @JavascriptInterface
        fun changerServeur() {
            runOnUiThread {
                AlertDialog.Builder(this@ActivitePrincipale)
                    .setTitle(R.string.changer_serveur)
                    .setMessage("L'application reviendra à l'écran de configuration. " +
                        "Vos saisies non transmises seront perdues.")
                    .setPositiveButton(R.string.changer_serveur) { _, _ ->
                        reglages.oublierServeur()
                        vueWeb.clearHistory()
                        afficherConfiguration()
                    }
                    .setNegativeButton("Annuler", null)
                    .show()
            }
        }
    }

    private fun lancerScanner() {
        scanner.launch(ScanOptions().apply {
            setDesiredBarcodeFormats(ScanOptions.QR_CODE)
            setPrompt(getString(R.string.scan_consigne))
            setBeepEnabled(true)
            setOrientationLocked(false)
        })
    }

    private fun renvoyerAuJS(valeurJS: String) {
        vueWeb.evaluateJavascript(
            "window.__vlpmScanResultat && window.__vlpmScanResultat($valeurJS);", null)
    }

    /** Échappe une valeur scannée avant de l'injecter dans du JavaScript. */
    private fun echapperPourJS(valeur: String): String {
        val echappe = valeur
            .replace("\\", "\\\\")
            .replace("'", "\\'")
            .replace("\"", "\\\"")
            .replace("\n", "\\n")
            .replace("\r", "")
            .replace("<", "\\u003c")
        return "'$echappe'"
    }

    override fun onDestroy() {
        travailleur.shutdown()
        super.onDestroy()
    }
}
