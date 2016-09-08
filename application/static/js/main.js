//$(document).ready(function(){
$(function(){
    $.ajaxSettings.traditional = true;
    var post_create_form = $("#post_create");
    post_create_form.hide();
    $("#togglePostFormLink").click(function(){
        post_create_form.slideToggle(200);
        event.preventDefault();
    });
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
        console.log(url, description, tags);
        $.ajax({
            type: "POST",
            url: "/api/post_create",
            data: {
                url: url,
                description: description,
                tag: tags,
                //day: "2016-08-20"
            },
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
