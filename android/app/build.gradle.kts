plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "fr.assopmf.vlpm"
    compileSdk = 35

    defaultConfig {
        applicationId = "fr.assopmf.vlpm"
        // Android 8 : couvre les terminaux encore en service dans les
        // collectivités, où le renouvellement du parc est lent.
        minSdk = 26
        targetSdk = 35
        // Version fournie par la chaîne de publication, dérivée de l'étiquette
        // git : l'APK et les exécutables serveur d'une même release portent
        // ainsi le même numéro. Valeurs par défaut pour une compilation locale.
        versionCode = (project.findProperty("versionCode") as String?)?.toInt() ?: 1
        versionName = (project.findProperty("versionName") as String?) ?: "0.0.0-local"
        resourceConfigurations += listOf("fr")
    }

    // Trousseau de publication, fourni par l'environnement et jamais versionné.
    // Sans lui, une mise à jour ne peut pas s'installer par-dessus la version
    // précédente : Android exige la même signature, et désinstaller efface les
    // saisies locales des agents.
    val trousseau = System.getenv("VLPM_TROUSSEAU")
    val trousseauDisponible = !trousseau.isNullOrBlank() && file(trousseau).exists()

    signingConfigs {
        if (trousseauDisponible) {
            create("publication") {
                storeFile = file(trousseau!!)
                storePassword = System.getenv("VLPM_TROUSSEAU_MOT_DE_PASSE")
                keyAlias = System.getenv("VLPM_TROUSSEAU_ALIAS") ?: "vlpm"
                keyPassword = System.getenv("VLPM_CLE_MOT_DE_PASSE")
                    ?: System.getenv("VLPM_TROUSSEAU_MOT_DE_PASSE")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // À défaut de trousseau, la signature de débogage permet au moins
            // de produire un APK d'essai. Il ne doit pas être distribué : la
            // clé de débogage diffère d'une machine à l'autre, et sur un
            // serveur d'intégration continue elle est régénérée à chaque fois.
            signingConfig = if (trousseauDisponible) {
                signingConfigs.getByName("publication")
            } else {
                signingConfigs.getByName("debug")
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures { buildConfig = true }
}

dependencies {
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.activity:activity-ktx:1.9.3")
    implementation("androidx.webkit:webkit:1.12.1")
    // Stockage chiffré du jeton et de l'adresse du serveur, plutôt que
    // localStorage lisible par quiconque accède au terminal.
    implementation("androidx.security:security-crypto:1.1.0-alpha06")
    // ZXing plutôt que ML Kit : pas de dépendance aux services Google, donc
    // l'application fonctionne aussi sur un terminal dégooglisé, ce qui
    // compte pour un outil de service public.
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
}
