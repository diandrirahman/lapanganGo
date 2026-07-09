package email

import "fmt"

func GenerateStaffInviteBody(params InviteEmailParams) string {
	venueText := "kami"
	if params.VenueName != "" {
		venueText = params.VenueName
	}

	return fmt.Sprintf(`Halo %s,

Anda telah diundang untuk menjadi bagian dari staf pengelola %s di LapangGo.
Untuk mengaktifkan akun Anda dan mengatur password baru, silakan klik tautan di bawah ini:

%s

Tautan ini hanya berlaku sekali. Jika Anda memiliki pertanyaan, silakan hubungi pihak pengelola terkait.

Terima kasih,
Tim LapangGo
`, params.StaffName, venueText, params.InviteURL)
}

func GenerateStaffPasswordResetBody(params ResetEmailParams) string {
	return fmt.Sprintf(`Halo %s,

Kami menerima permintaan untuk melakukan reset password akun LapangGo Anda.
Jika ini benar Anda, silakan klik tautan di bawah ini untuk mengatur ulang password:

%s

Tautan ini akan kedaluwarsa sesuai waktu yang ditentukan. Jika Anda tidak pernah meminta pengaturan ulang password, Anda dapat mengabaikan email ini.

Terima kasih,
Tim LapangGo
`, params.StaffName, params.ResetURL)
}
