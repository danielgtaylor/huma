import uuid

def value_str(value):
    s = str(value)
    if isinstance(value, bool):
        s = s.lower()
    return s

def define_env(env):
    """
    This is the hook for defining variables, macros and filters
    """

    @env.macro
    def asciinema(file, **kwargs):
        html = ""
        opts = {
            "autoPlay": True,
            "controls": "'auto'",
            "loop": False,
            "theme": "'nord'",
            "terminalLineHeight": 1.4,
        }

        # Overwrite defaults with kwargs
        for key, value in kwargs.items():
            opts[key] = value

        # Create an empty div that we will use for the player
        div_id = "asciinema-" + str(uuid.uuid4())
        div_style = "z-index: 1; position: relative;"
        html += '<div id="' + div_id + '" style="' + div_style + '"></div>'

        # Define JS representing creating the player
        create_player_js = ""
        create_player_js += "AsciinemaPlayer.create('" + file + "', document.getElementById('" + div_id + "'), {"
        for key, value in opts.items():
            create_player_js += '"' + key + '": ' + value_str(value) + ','
        create_player_js += "});"

        # Create script tag that will perform cast by either registering for the DOM to
        # load or firing immediately if already loaded
        html += "<script>"
        html += "if (document.readyState === 'loading') {"
        html += "document.addEventListener('DOMContentLoaded', function() {"
        html += create_player_js
        html += "});"
        html += "} else {"
        html += create_player_js
        html += "}"
        html += "</script>"

        return html
