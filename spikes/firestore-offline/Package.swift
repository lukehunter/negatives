// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "FirestoreOfflineSpike",
    platforms: [.macOS(.v14)],
    dependencies: [
        .package(url: "https://github.com/firebase/firebase-ios-sdk.git", from: "13.1.0"),
        .package(url: "https://github.com/google/GoogleSignIn-iOS.git", from: "10.1.0"),
    ],
    targets: [
        .executableTarget(
            name: "FirestoreOfflineSpike",
            dependencies: [
                .product(name: "FirebaseAuth", package: "firebase-ios-sdk"),
                .product(name: "FirebaseFirestore", package: "firebase-ios-sdk"),
                .product(name: "GoogleSignIn", package: "GoogleSignIn-iOS"),
            ]
        ),
    ]
)
