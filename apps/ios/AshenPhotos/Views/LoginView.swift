import SwiftUI

struct LoginView: View {
    @EnvironmentObject private var auth: AuthStore
    @State private var email = ""
    @State private var password = ""
    @State private var isRegister = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("Email", text: $email)
                        .textInputAutocapitalization(.never)
                        .keyboardType(.emailAddress)
                        .autocorrectionDisabled()
                    SecureField("Password", text: $password)
                }
                if let err = auth.errorMessage {
                    Text(err).foregroundStyle(.red).font(.footnote)
                }
                Section {
                    Button(isRegister ? "Create account" : "Log in") {
                        Task {
                            if isRegister { await auth.register(email: email, password: password) }
                            else { await auth.login(email: email, password: password) }
                        }
                    }
                    .disabled(auth.busy || email.isEmpty || password.count < 8)

                    Button(isRegister ? "Have an account? Log in" : "New here? Create account") {
                        isRegister.toggle()
                    }
                    .font(.footnote)
                }
            }
            .navigationTitle("Ashen Photos")
            .overlay { if auth.busy { ProgressView() } }
        }
    }
}
