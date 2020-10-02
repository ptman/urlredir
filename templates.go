// Copyright (c) 2017-2020 Paul Tötterman <ptman@iki.fi>. All rights reserved.

package main

const adminPage = `
<html>
<head>
<title>URL Shortener</title>
<script type="text/javascript">
function deleteLink(name) {
	var xhr = window.XMLHttpRequest ? new XMLHttpRequest() :
		new ActiveXObject('Microsoft.HTTP');
	xhr.open('DELETE', '/' + name);
	xhr.onreadystatechange = function() {
		if (xhr.readyState > 3 && xhr.status == 200) {
			window.location.href = '/_admin';
		}
	};
	xhr.send();
}
</script>
</head>
<body>
<p>
<form action="{{.path}}" method="post">
<input name="name" id="name" placeholder="name">
<input name="url" id="url" placeholder="https://...">
<input name="user" id="user" placeholder="username" value="{{.user}}">
<input type="submit" value="Add">
</form>
</p>
<p>
<ul>
{{range .urls}}
<li>
<a href="/{{.name}}">{{.name}}</a>
<a href="{{.url}}">{{.url}}</a>
{{.hits}}
<a href="#" onclick="deleteLink('{{.name}}');">Delete</a>
</li>
{{end}}
</ul>
</p>
</body>
</html>
`
