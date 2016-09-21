//$(document).ready(function(){
$(function(){
    function setCookie(key, value) {
        var expires = new Date();
        expires.setTime(expires.getTime() + 86400000*7); //1 week  
        document.cookie = key + '=' + value + ';expires=' + expires.toUTCString();
    }

    function getCookie(key) {
        var keyValue = document.cookie.match('(^|;) ?' + key + '=([^;]*)(;|$)');
        return keyValue ? keyValue[2] : null;
    }  

    $.ajaxSettings.traditional = true;
    var post_create_form = $("#post_create");
    post_create_form.hide();
    $("#togglePostFormLink").click(function(){
        post_create_form.slideToggle(200);
        event.preventDefault();
    });
    $("#post_create").find('#form_hidden')[0].checked = getCookie('hidden');
    post_create_form.submit(function( event ) {
        var url = $("#post_create").find('input[name="url"]').val();
        if (!url) {
            alert("Fill URL field.");
            event.preventDefault();
            return;
        }
        var description = $("#post_create").find('textarea[name="description"]').val();
        if (!description) {
            alert("Fill Description field.");
            event.preventDefault();
            return;
        }
        var tags_str = $("#post_create").find('input[name="tags"]').val();
        var tags = tags_str.split(',');
        for(var i = 0; i < tags.length; i++) {
            tags[i] = tags[i].replace(/^\s*/, "").replace(/\s*$/, "");
            if (!tags[i]) {         
                tags.splice(i, 1);
                i--;
            }
        }
        var date = $("#post_create").find('input[name="date"]').val();
        var prms = {
                url: url,
                description: description,
                tag: tags,
                //date: "2016-08-20"
            };
        if (date) {
            prms['date'] = date;
        }
        var hidden = $("#post_create").find('#form_hidden')[0].checked;
        if (hidden) {
            prms['hidden'] = true;
            setCookie('hidden', hidden);
        } else {
            setCookie('hidden', '');
        }
        $.ajax({
            type: "POST",
            url: "/api/post_create",
            data: prms,
            success: function(data) {
                console.log(data);
                if (data.status == 200) {
                    location.reload();
                } else {
                    alert(data.status + ': ' + data.message);
                }
            },
            error: function(xhr, status, error) {
                alert("Something going wrong.");
            }
        });
        event.preventDefault();
    });
});
