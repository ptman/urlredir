# Copyright (c) 2017 Paul Tötterman <ptman@iki.fi>. All rights reserved.
FROM scratch
MAINTAINER Paul Tötterman <paul.totterman@gmail.com>

COPY urlredir /

EXPOSE 8080
CMD ["/urlredir"]
