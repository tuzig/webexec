source = ["./dist/webexec_darwin_all/webexec", "./sh.webexec.daemon.tmpl", 
          "./replace_n_launch.sh"]
bundle_id = "sh.webexec"

apple_id {
  username = "benny@tuzig.com"
  password = "@env:AC_PASSWORD"
}

sign {
  application_identity = "050CFC9A30AB6E97218B66CFD66A6EFCDA60F707"
}

dmg {
  output_path = "./dist/webexec_0.15.1.dmg"
  volume_name = "webexec"
}
