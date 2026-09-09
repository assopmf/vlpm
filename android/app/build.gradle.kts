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
        versionCode = 1
        versionName = "1.0.0"
        resourceConfigurations += listOf("fr")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            // L'application n'est pas distribuée par un magasin : elle
            // s'installe par APK, ce qui exige une signature. À défaut de
            // trousseau fourni, on retombe sur la signature de débogage pour
            // que la compilation aboutisse quand même.
            signingConfig = signingConfigs.getByName("debug")
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
