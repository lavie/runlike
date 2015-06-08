    "See this docker container? I wish I could run another one just like it, 
    but I'll be damned if I'm going to type all those command-line switches manually!"

This is what `runlike` does. You give it a docker container, it outputs the command line necessary to run another one just like it, along with all those pesky options (ports, links, volumes, ...). It's a real time saver for those that normally deploy their docker containers via some CM tool like Ansible/Chef and then find themselves needing to manually re-run some container.

Usage:

    runlike container-name

This prints out what you need to run to get a similar container. You can do `$(runlike container-name)` to simply execute its output in one step.

`-p` breaks the command line down to nice, pretty lines.

    runlike -p container-name

